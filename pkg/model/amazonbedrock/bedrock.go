package amazonbedrock

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// DefaultModelID is the default Bedrock model to use if none specified.
const DefaultModelID = "anthropic.claude-3-5-sonnet-20241022-v2:0"

// Client is an interface representing the Bedrock Runtime client.
// This abstraction allows for easier testing and mocking.
type Client interface {
	Converse(
		ctx context.Context,
		params *bedrockruntime.ConverseInput,
		optFns ...func(*bedrockruntime.Options),
	) (*bedrockruntime.ConverseOutput, error)

	ConverseStream(
		ctx context.Context,
		params *bedrockruntime.ConverseStreamInput,
		optFns ...func(*bedrockruntime.Options),
	) (*bedrockruntime.ConverseStreamOutput, error)
}

// Option is a function that configures model options.
type Option func(*Options)

// Options holds configuration for the Bedrock model.
type Options struct {
	// ModelID is the Amazon Bedrock model identifier
	ModelID string

	// Temperature controls randomness (0.0-1.0)
	Temperature float32

	// MaxTokens limits the response length
	MaxTokens int32

	// TopP controls nucleus sampling (0.0-1.0)
	TopP float32
}

// WithModelID sets the Bedrock model ID to use.
func WithModelID(modelID string) Option {
	return func(o *Options) {
		o.ModelID = modelID
	}
}

// WithTemperature sets the sampling temperature.
func WithTemperature(temperature float32) Option {
	return func(o *Options) {
		o.Temperature = temperature
	}
}

// WithMaxTokens sets the maximum tokens to generate.
func WithMaxTokens(maxTokens int32) Option {
	return func(o *Options) {
		o.MaxTokens = maxTokens
	}
}

// WithTopP sets the top-p (nucleus) sampling parameter.
func WithTopP(topP float32) Option {
	return func(o *Options) {
		o.TopP = topP
	}
}

// Model implements the model.Model interface for Amazon Bedrock.
type Model struct {
	client Client
	opts   Options
}

// NewModel creates a new Bedrock model instance.
func NewModel(client Client, optFns ...Option) *Model {
	opts := Options{
		ModelID:   DefaultModelID,
		MaxTokens: 4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Model{
		client: client,
		opts:   opts,
	}
}

// Generate executes a content generation request against Amazon Bedrock.
// Returns an iterator that yields Response chunks as they are received.
func (m *Model) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		if req == nil || len(req.Messages) == 0 {
			yield(nil, ErrNoMessages)
			return
		}

		messages, systemPrompt := convertMessagesToBedrock(req.Messages)

		// Build inference configuration
		inferenceConfig := &types.InferenceConfiguration{
			MaxTokens: aws.Int32(m.opts.MaxTokens),
		}
		if m.opts.Temperature > 0 {
			inferenceConfig.Temperature = aws.Float32(m.opts.Temperature)
		}
		if m.opts.TopP > 0 {
			inferenceConfig.TopP = aws.Float32(m.opts.TopP)
		}

		// Build system prompt (combine request system prompt + extracted system text)
		var systemContent []types.SystemContentBlock
		var systemParts []string
		if req.SystemPrompt != "" {
			systemParts = append(systemParts, req.SystemPrompt)
		}
		if systemPrompt != "" {
			systemParts = append(systemParts, systemPrompt)
		}
		if len(systemParts) > 0 {
			systemContent = []types.SystemContentBlock{
				&types.SystemContentBlockMemberText{
					Value: strings.Join(systemParts, "\n\n"),
				},
			}
		}

		// Convert tools if provided
		var toolConfig *types.ToolConfiguration
		if len(req.Tools) > 0 {
			toolConfig = convertToolsToBedrock(req.Tools)
		}

		if req.Stream {
			m.streamingGenerate(ctx, messages, systemContent, inferenceConfig, toolConfig, yield)
			return
		}

		// Non-streaming mode
		input := &bedrockruntime.ConverseInput{
			ModelId:         aws.String(m.opts.ModelID),
			Messages:        messages,
			InferenceConfig: inferenceConfig,
			System:          systemContent,
			ToolConfig:      toolConfig,
		}

		output, err := m.client.Converse(ctx, input)
		if err != nil {
			yield(nil, fmt.Errorf("bedrock converse: %w", err))
			return
		}

		msg := convertBedrockOutputToMessage(output)

		resp := &model.Response{
			Message:      msg,
			FinishReason: string(output.StopReason),
			Usage:        convertUsage(output.Usage),
			Partial:      false,
		}

		yield(resp, nil)
	}
}

// streamingGenerate handles streaming responses from Bedrock ConverseStream API.
func (m *Model) streamingGenerate(
	ctx context.Context,
	messages []types.Message,
	system []types.SystemContentBlock,
	inferenceConfig *types.InferenceConfiguration,
	toolConfig *types.ToolConfiguration,
	yield func(*model.Response, error) bool,
) {
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(m.opts.ModelID),
		Messages:        messages,
		InferenceConfig: inferenceConfig,
		System:          system,
		ToolConfig:      toolConfig,
	}

	output, err := m.client.ConverseStream(ctx, input)
	if err != nil {
		yield(nil, fmt.Errorf("bedrock converse stream: %w", err))
		return
	}

	stream := output.GetStream()
	defer func() { _ = stream.Close() }()

	state := &streamState{}
	m.processStreamEvents(stream, state, yield)

	if err := stream.Err(); err != nil {
		yield(nil, fmt.Errorf("bedrock stream error: %w", err))
		return
	}

	m.yieldFinalResponse(state, yield)
}

// streamState holds the accumulated state during streaming.
type streamState struct {
	textBuffer     strings.Builder
	toolCalls      []message.ToolCall
	currentToolUse *toolUseBuilder
	stopReason     string
	usage          *model.UsageInfo
}

// processStreamEvents processes all events from the stream.
func (m *Model) processStreamEvents(
	stream *bedrockruntime.ConverseStreamEventStream,
	state *streamState,
	yield func(*model.Response, error) bool,
) {
	for event := range stream.Events() {
		if !m.handleStreamEvent(event, state, yield) {
			return
		}
	}
}

// handleStreamEvent handles a single stream event. Returns false if iteration should stop.
func (m *Model) handleStreamEvent(
	event types.ConverseStreamOutput,
	state *streamState,
	yield func(*model.Response, error) bool,
) bool {
	switch e := event.(type) {
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		return m.handleContentBlockDelta(e, state, yield)
	case *types.ConverseStreamOutputMemberContentBlockStart:
		if start, ok := e.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
			state.currentToolUse = &toolUseBuilder{
				id:        aws.ToString(start.Value.ToolUseId),
				name:      aws.ToString(start.Value.Name),
				inputJSON: "",
			}
		}
	case *types.ConverseStreamOutputMemberContentBlockStop:
		if state.currentToolUse != nil {
			argsJSON := state.currentToolUse.inputJSON
			if argsJSON == "" {
				argsJSON = "{}"
			}
			state.toolCalls = append(state.toolCalls, message.ToolCall{
				ID:        state.currentToolUse.id,
				Name:      state.currentToolUse.name,
				Type:      "function",
				Arguments: argsJSON,
			})
			state.currentToolUse = nil
		}
	case *types.ConverseStreamOutputMemberMessageStop:
		state.stopReason = string(e.Value.StopReason)
	case *types.ConverseStreamOutputMemberMetadata:
		state.usage = convertUsage(e.Value.Usage)
	}
	return true
}

// handleContentBlockDelta handles content block delta events.
func (m *Model) handleContentBlockDelta(
	e *types.ConverseStreamOutputMemberContentBlockDelta,
	state *streamState,
	yield func(*model.Response, error) bool,
) bool {
	switch delta := e.Value.Delta.(type) {
	case *types.ContentBlockDeltaMemberText:
		state.textBuffer.WriteString(delta.Value)
		resp := &model.Response{
			Message: message.NewAIMessageFromText(delta.Value),
			Partial: true,
		}
		return yield(resp, nil)
	case *types.ContentBlockDeltaMemberToolUse:
		if state.currentToolUse != nil {
			state.currentToolUse.inputJSON += aws.ToString(delta.Value.Input)
		}
	}
	return true
}

// yieldFinalResponse yields the final complete response.
func (m *Model) yieldFinalResponse(state *streamState, yield func(*model.Response, error) bool) {
	var finalMsg message.Message
	if len(state.toolCalls) > 0 {
		aiMsg := message.NewAIMessage(message.Parts{
			message.TextPart{Text: state.textBuffer.String()},
		})
		aiMsg.ToolCalls = state.toolCalls
		finalMsg = aiMsg
	} else {
		finalMsg = message.NewAIMessageFromText(state.textBuffer.String())
	}

	resp := &model.Response{
		Message:      finalMsg,
		FinishReason: state.stopReason,
		Usage:        state.usage,
		Partial:      false,
	}
	yield(resp, nil)
}

// toolUseBuilder accumulates tool use data during streaming.
type toolUseBuilder struct {
	id        string
	name      string
	inputJSON string
}

// Capabilities returns the features supported by this Bedrock model.
func (m *Model) Capabilities() model.Capabilities {
	// Most capabilities depend on the specific model being used
	return model.Capabilities{
		Streaming:           true,
		Tools:               m.supportsTools(),
		StructuredOutput:    false, // Bedrock Converse API doesn't have native structured output
		Vision:              m.supportsVision(),
		MaxContextTokens:    m.getMaxContextTokens(),
		MaxOutputTokens:     int(m.opts.MaxTokens),
		SupportedModalities: m.getSupportedModalities(),
	}
}

// supportsTools checks if the model supports tool calling.
func (m *Model) supportsTools() bool {
	modelID := m.opts.ModelID
	// Claude models support tools
	if strings.Contains(modelID, "anthropic.claude") {
		return true
	}
	// Mistral models support tools
	if strings.Contains(modelID, "mistral") {
		return true
	}
	// Amazon Nova models support tools
	if strings.Contains(modelID, "amazon.nova") {
		return true
	}
	// Cohere Command R models support tools
	if strings.Contains(modelID, "cohere.command") {
		return true
	}
	// Meta Llama 3.1+ models support tools
	if strings.Contains(modelID, "meta.llama3-1") || strings.Contains(modelID, "meta.llama3-2") {
		return true
	}
	return false
}

// supportsVision checks if the model supports image inputs.
func (m *Model) supportsVision() bool {
	modelID := m.opts.ModelID
	// Claude 3+ models support vision
	if strings.Contains(modelID, "anthropic.claude-3") {
		return true
	}
	// Amazon Nova models support vision
	if strings.Contains(modelID, "amazon.nova") {
		return true
	}
	return false
}

// getMaxContextTokens returns the context window size based on model.
func (m *Model) getMaxContextTokens() int {
	modelID := m.opts.ModelID
	switch {
	case strings.Contains(modelID, "claude-3-5"):
		return 200000
	case strings.Contains(modelID, "claude-3"):
		return 200000
	case strings.Contains(modelID, "mistral-large"):
		return 128000
	case strings.Contains(modelID, "llama3-70b"):
		return 128000
	case strings.Contains(modelID, "titan-text"):
		return 32000
	default:
		return 100000 // Conservative default
	}
}

// getSupportedModalities returns supported input modalities.
func (m *Model) getSupportedModalities() []string {
	if m.supportsVision() {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

// Ensure Model implements model.Model interface
var _ model.Model = (*Model)(nil)
