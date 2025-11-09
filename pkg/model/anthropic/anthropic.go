package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// Client defines the interface for interacting with the Anthropic API.
type Client interface {
	Messages() *anthropic.MessageService
}

// ClientWrapper wraps the Anthropic SDK client.
type ClientWrapper struct {
	inner *anthropic.Client
}

// NewClientWrapper creates a new ClientWrapper.
func NewClientWrapper(client *anthropic.Client) *ClientWrapper {
	return &ClientWrapper{inner: client}
}

// Messages returns the messages service.
func (c *ClientWrapper) Messages() *anthropic.MessageService {
	return &c.inner.Messages
}

// Options configures the Anthropic model.
type Options struct {
	model       string
	maxTokens   int64
	temperature float64
	apiKey      string
	tools       []tool.Tool
}

// Model implements the model.Model interface for Anthropic Claude.
type Model struct {
	client Client
	opts   Options
}

// NewModel creates a new Anthropic model with the given options.
func NewModel(optFns ...func(o *Options)) *Model {
	opts := Options{
		model:       string(anthropic.ModelClaudeSonnet4_0),
		maxTokens:   4096,
		temperature: 0.7,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	clientOpts := []option.RequestOption{}
	if opts.apiKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.apiKey))
	}

	client := anthropic.NewClient(clientOpts...)

	return &Model{
		client: &ClientWrapper{inner: &client},
		opts:   opts,
	}
}

// NewModelFromClient creates a model from a custom client (for testing).
func NewModelFromClient(client Client, optFns ...func(o *Options)) *Model {
	opts := Options{
		model:       string(anthropic.ModelClaudeSonnet4_0),
		maxTokens:   4096,
		temperature: 0.7,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Model{
		client: client,
		opts:   opts,
	}
}

// WithModel returns an option function to set the model name.
func WithModel(modelName string) func(o *Options) {
	return func(o *Options) {
		o.model = modelName
	}
}

// WithTemperature returns an option function to set the temperature.
// Temperature controls randomness in the output (0.0 to 1.0).
func WithTemperature(temperature float64) func(o *Options) {
	return func(o *Options) {
		o.temperature = temperature
	}
}

// WithMaxTokens returns an option function to set the maximum output tokens.
func WithMaxTokens(maxTokens int64) func(o *Options) {
	return func(o *Options) {
		o.maxTokens = maxTokens
	}
}

// WithAPIKey returns an option function to set the API key.
func WithAPIKey(apiKey string) func(o *Options) {
	return func(o *Options) {
		o.apiKey = apiKey
	}
}

// BindTools returns a new model configured with the provided tools.
func (m *Model) BindTools(tools ...tool.Tool) model.Model {
	return NewModelFromClient(m.client, func(o *Options) {
		*o = m.opts
		o.tools = normalizeTools(tools)
	})
}

// Generate executes a message request against the Anthropic API.
// Returns an iterator that yields messages as they are received.
// For streaming, multiple intermediate messages are yielded followed by the final complete message.
// For non-streaming (blocking), only the final message is yielded.
func (m *Model) Generate(ctx context.Context, msgs []message.Message) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		if len(msgs) == 0 {
			yield(nil, fmt.Errorf("generate requires at least one message"))
			return
		}

		converted, systemText := convertMessagesToAnthropic(msgs)

		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(m.opts.model),
			Messages:  converted,
			MaxTokens: m.opts.maxTokens,
		}

		if systemText != "" {
			params.System = []anthropic.TextBlockParam{
				{Text: systemText},
			}
		}

		if m.opts.temperature > 0 {
			params.Temperature = param.NewOpt(m.opts.temperature)
		}

		if len(m.opts.tools) > 0 {
			params.Tools = convertToolsToAnthropic(m.opts.tools)
		}

		// Try streaming first
		stream := m.client.Messages().NewStreaming(ctx, params)
		if stream.Err() == nil {
			// Streaming successful
			m.streamGenerate(stream, yield)
			return
		}

		// Fall back to non-streaming
		response, err := m.client.Messages().New(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}

		msg, err := convertAnthropicResponseToMessage(response)
		if err != nil {
			yield(nil, err)
			return
		}

		yield(msg, nil)
	}
}

// streamGenerate handles streaming responses from Anthropic API
func (m *Model) streamGenerate(
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion],
	yield func(message.Message, error) bool,
) {
	defer func() { _ = stream.Close() }() // Best effort close

	var textBuffer strings.Builder
	var toolCalls []message.ToolCall

	for stream.Next() {
		event := stream.Current()

		switch e := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			if delta, ok := e.Delta.AsAny().(anthropic.TextDelta); ok {
				textBuffer.WriteString(delta.Text)
				aiMsg := message.NewAIMessageFromText(delta.Text)
				if !yield(aiMsg, nil) {
					return
				}
			}

		case anthropic.ContentBlockStartEvent:
			if toolUse, ok := e.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
				var inputMap map[string]any
				if err := json.Unmarshal([]byte(toolUse.JSON.Input.Raw()), &inputMap); err == nil {
					toolCalls = append(toolCalls, message.ToolCall{
						ID:        toolUse.ID,
						Name:      toolUse.Name,
						Type:      "function",
						Arguments: inputMap,
					})
				}
			}

		case anthropic.MessageStopEvent:
			// Send final message with accumulated content
			var parts message.Parts
			if textBuffer.Len() > 0 {
				parts = message.Parts{message.NewTextPart(textBuffer.String())}
			}

			finalMsg := message.NewAIMessage(parts)
			if len(toolCalls) > 0 {
				finalMsg.ToolCalls = toolCalls
			}

			if len(parts) > 0 || len(toolCalls) > 0 {
				yield(finalMsg, nil)
			}
			return
		}
	}

	if err := stream.Err(); err != nil {
		yield(nil, err)
	}
}

// Helper functions

func normalizeTools(tools []tool.Tool) []tool.Tool {
	if tools == nil {
		return nil
	}
	result := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if t != nil {
			result = append(result, t)
		}
	}
	return result
}

//nolint:gocyclo // Message conversion requires handling many part types
func convertMessagesToAnthropic(msgs []message.Message) ([]anthropic.MessageParam, string) {
	var apiMessages []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range msgs {
		switch msg.Type() {
		case message.TypeSystem:
			parts := msg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					systemPrompt = textPart.Text
				}
			}

		case message.TypeHuman:
			var content []anthropic.ContentBlockParamUnion
			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					content = append(content, anthropic.NewTextBlock(textPart.Text))
				}
			}
			if len(content) > 0 {
				apiMessages = append(apiMessages, anthropic.NewUserMessage(content...))
			}

		case message.TypeAI:
			aiMsg, ok := msg.(*message.AIMessage)
			if !ok {
				continue
			}

			var content []anthropic.ContentBlockParamUnion

			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					content = append(content, anthropic.NewTextBlock(textPart.Text))
				}
			}

			for _, tc := range aiMsg.ToolCalls {
				inputJSON, _ := json.Marshal(tc.Arguments)
				content = append(content, anthropic.NewToolUseBlock(tc.ID, tc.Name, string(inputJSON)))
			}

			if len(content) > 0 {
				apiMessages = append(apiMessages, anthropic.NewAssistantMessage(content...))
			}

		case message.TypeTool:
			toolMsg, ok := msg.(*message.ToolMessage)
			if !ok {
				continue
			}

			var resultStr string
			parts := toolMsg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					resultStr = textPart.Text
				}
			}
			if resultStr == "" {
				resultStr = fmt.Sprintf("%v", parts)
			}

			apiMessages = append(apiMessages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(toolMsg.ToolCallID, resultStr, false),
			))
		}
	}

	return apiMessages, systemPrompt
}

func convertToolsToAnthropic(tools []tool.Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()

		schemaJSON, err := json.Marshal(def.Function.Parameters)
		if err != nil {
			continue
		}

		var schemaMap map[string]any
		if err := json.Unmarshal(schemaJSON, &schemaMap); err != nil {
			continue
		}

		toolParam := anthropic.ToolParam{
			Name:        def.Function.Name,
			Description: param.NewOpt(def.Function.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: schemaMap,
			},
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &toolParam})
	}

	return result
}

func convertAnthropicResponseToMessage(resp *anthropic.Message) (message.Message, error) {
	var textParts []message.Part
	var toolCalls []message.ToolCall

	for i := range resp.Content {
		switch b := resp.Content[i].AsAny().(type) {
		case anthropic.TextBlock:
			textParts = append(textParts, message.TextPart{Text: b.Text})

		case anthropic.ToolUseBlock:
			var inputMap map[string]any
			if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &inputMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool input: %w", err)
			}

			toolCalls = append(toolCalls, message.ToolCall{
				ID:        b.ID,
				Name:      b.Name,
				Type:      "function",
				Arguments: inputMap,
			})
		}
	}

	aiMsg := message.NewAIMessage(textParts)
	if len(toolCalls) > 0 {
		aiMsg.ToolCalls = toolCalls
	}

	if len(textParts) == 0 && len(toolCalls) == 0 {
		return nil, fmt.Errorf("anthropic response contained no content")
	}

	return aiMsg, nil
}

// Compile-time interface checks
var (
	_ model.Model     = (*Model)(nil)
	_ model.ToolAware = (*Model)(nil)
)
