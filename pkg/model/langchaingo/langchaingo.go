package langchaingo

import (
	"context"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/tmc/langchaingo/llms"
)

// Options configures the LangChainGo model adapter behavior.
type Options struct {
	// Temperature controls randomness in output (0.0 to 1.0).
	// Higher values produce more random output.
	Temperature float64

	// MaxTokens is the maximum number of tokens to generate.
	MaxTokens int

	// StopWords are sequences that will stop generation when encountered.
	StopWords []string

	// Streaming enables streaming mode when true.
	Streaming bool
}

// Option is a function that configures Options.
type Option func(*Options)

// WithTemperature sets the temperature for generation.
func WithTemperature(temperature float64) Option {
	return func(o *Options) {
		o.Temperature = temperature
	}
}

// WithMaxTokens sets the maximum tokens to generate.
func WithMaxTokens(maxTokens int) Option {
	return func(o *Options) {
		o.MaxTokens = maxTokens
	}
}

// WithStopWords sets the stop sequences for generation.
func WithStopWords(stopWords ...string) Option {
	return func(o *Options) {
		o.StopWords = stopWords
	}
}

// WithStreaming enables or disables streaming mode.
func WithStreaming(enabled bool) Option {
	return func(o *Options) {
		o.Streaming = enabled
	}
}

// Model wraps a LangChainGo llms.Model to implement the AgentMesh model.Model interface.
type Model struct {
	llm   llms.Model
	opts  Options
	tools []tool.Tool
}

// NewModel creates a new AgentMesh model adapter from a LangChainGo model.
// Returns an error if the llm parameter is nil.
func NewModel(llm llms.Model, optFns ...Option) (*Model, error) {
	if err := validate.NotNil(llm, "langchaingo: llm"); err != nil {
		return nil, err
	}

	opts := Options{
		Temperature: 0.7,
		MaxTokens:   4096,
		Streaming:   false,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Model{
		llm:  llm,
		opts: opts,
	}, nil
}

// MustNewModel creates a new model adapter, panicking on error.
// Use this only when you can guarantee the llm is non-nil.
func MustNewModel(llm llms.Model, optFns ...Option) *Model {
	m, err := NewModel(llm, optFns...)
	if err != nil {
		panic(err)
	}
	return m
}

// BindTools returns a new model with the specified tools bound for function calling.
func (m *Model) BindTools(tools ...tool.Tool) *Model {
	return &Model{
		llm:   m.llm,
		opts:  m.opts,
		tools: tools,
	}
}

// Capabilities returns the features supported by this model adapter.
// Note: LangChainGo doesn't expose capability introspection, so we return
// conservative defaults. Streaming support depends on the underlying model.
func (m *Model) Capabilities() model.Capabilities {
	return model.Capabilities{
		Streaming:           m.opts.Streaming,
		Tools:               true, // Assume tools support; will error at runtime if not supported
		StructuredOutput:    false,
		NativeReasoning:     false,
		Logprobs:            false,
		Vision:              false, // Conservative; LangChainGo doesn't expose this
		Audio:               false,
		MaxContextTokens:    0, // Unknown
		MaxOutputTokens:     m.opts.MaxTokens,
		SupportedModalities: []string{"text"},
	}
}

// Generate executes a generation request against the wrapped LangChainGo model.
// Returns an iterator that yields model.Response as generation progresses.
func (m *Model) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		if req == nil || len(req.Messages) == 0 {
			yield(nil, fmt.Errorf("langchaingo: generate requires at least one message"))
			return
		}

		// Convert AgentMesh messages to LangChainGo format
		lcMessages := m.convertMessages(req)

		// Build call options
		callOpts := m.buildCallOptions(req)

		// Call the LangChainGo model
		resp, err := m.llm.GenerateContent(ctx, lcMessages, callOpts...)
		if err != nil {
			yield(nil, fmt.Errorf("langchaingo: generation failed: %w", err))
			return
		}

		// Convert response
		response, err := m.convertResponse(resp)
		if err != nil {
			yield(nil, fmt.Errorf("langchaingo: failed to convert response: %w", err))
			return
		}

		yield(response, nil)
	}
}

// convertMessages converts AgentMesh messages to LangChainGo MessageContent format.
func (m *Model) convertMessages(req *model.Request) []llms.MessageContent {
	messages := req.Messages

	// Prepend system prompt if provided
	if req.SystemPrompt != "" {
		systemMsg := message.NewSystemMessageFromText(req.SystemPrompt)
		messages = append([]message.Message{systemMsg}, messages...)
	}

	result := make([]llms.MessageContent, 0, len(messages))

	for _, msg := range messages {
		lcMsg := m.convertMessage(msg)
		result = append(result, lcMsg)
	}

	return result
}

// convertMessage converts a single AgentMesh message to LangChainGo format.
func (m *Model) convertMessage(msg message.Message) llms.MessageContent {
	role := m.convertRole(msg.Type())

	// Handle ToolMessage specially - LangChainGo expects ToolCallResponse format
	if toolMsg, ok := msg.(*message.ToolMessage); ok {
		var content string
		for _, part := range toolMsg.Parts() {
			if text, ok := part.(message.TextPart); ok {
				content = text.Text
				break
			}
		}
		return llms.MessageContent{
			Role: role,
			Parts: []llms.ContentPart{
				llms.ToolCallResponse{
					ToolCallID: toolMsg.ToolCallID,
					Content:    content,
				},
			},
		}
	}

	parts := m.convertParts(msg.Parts())

	return llms.MessageContent{
		Role:  role,
		Parts: parts,
	}
}

// convertRole maps AgentMesh message types to LangChainGo ChatMessageType.
func (m *Model) convertRole(t message.Type) llms.ChatMessageType {
	switch t {
	case message.TypeSystem:
		return llms.ChatMessageTypeSystem
	case message.TypeHuman:
		return llms.ChatMessageTypeHuman
	case message.TypeAI:
		return llms.ChatMessageTypeAI
	case message.TypeTool:
		return llms.ChatMessageTypeTool
	case message.TypeFunction:
		return llms.ChatMessageTypeFunction
	default:
		return llms.ChatMessageTypeHuman
	}
}

// convertParts converts AgentMesh message parts to LangChainGo ContentPart format.
func (m *Model) convertParts(parts []message.Part) []llms.ContentPart {
	result := make([]llms.ContentPart, 0, len(parts))

	for _, part := range parts {
		switch p := part.(type) {
		case message.TextPart:
			result = append(result, llms.TextContent{Text: p.Text})
		case *message.TextPart:
			result = append(result, llms.TextContent{Text: p.Text})
		case message.FunctionCallPart:
			if p.FunctionCall != nil {
				result = append(result, llms.ToolCall{
					ID:   p.FunctionCall.ID,
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: p.FunctionCall.Arguments,
					},
				})
			}
		case *message.FunctionCallPart:
			if p.FunctionCall != nil {
				result = append(result, llms.ToolCall{
					ID:   p.FunctionCall.ID,
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: p.FunctionCall.Arguments,
					},
				})
			}
		case message.FunctionResponsePart:
			if p.FunctionResponse != nil {
				content := ""
				if str, ok := p.FunctionResponse.Response.(string); ok {
					content = str
				}
				result = append(result, llms.ToolCallResponse{
					ToolCallID: p.FunctionResponse.ID,
					Name:       p.FunctionResponse.Name,
					Content:    content,
				})
			}
		case *message.FunctionResponsePart:
			if p.FunctionResponse != nil {
				content := ""
				if str, ok := p.FunctionResponse.Response.(string); ok {
					content = str
				}
				result = append(result, llms.ToolCallResponse{
					ToolCallID: p.FunctionResponse.ID,
					Name:       p.FunctionResponse.Name,
					Content:    content,
				})
			}
			// Skip unsupported part types
		}
	}

	return result
}

// buildCallOptions constructs LangChainGo call options from the request.
func (m *Model) buildCallOptions(req *model.Request) []llms.CallOption {
	opts := []llms.CallOption{
		llms.WithTemperature(m.opts.Temperature),
	}

	if m.opts.MaxTokens > 0 {
		opts = append(opts, llms.WithMaxTokens(m.opts.MaxTokens))
	}

	if len(m.opts.StopWords) > 0 {
		opts = append(opts, llms.WithStopWords(m.opts.StopWords))
	}

	// Add tools if bound
	if len(m.tools) > 0 || len(req.Tools) > 0 {
		allTools := make([]tool.Tool, 0, len(m.tools)+len(req.Tools))
		allTools = append(allTools, m.tools...)
		allTools = append(allTools, req.Tools...)
		lcTools := m.convertTools(allTools)
		if len(lcTools) > 0 {
			opts = append(opts, llms.WithTools(lcTools))
		}
	}

	return opts
}

// convertTools converts AgentMesh tools to LangChainGo tool definitions.
func (m *Model) convertTools(tools []tool.Tool) []llms.Tool {
	result := make([]llms.Tool, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()
		if def == nil || def.Function.Parameters == nil {
			continue
		}
		lcTool := llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  def.Function.Parameters,
			},
		}
		result = append(result, lcTool)
	}

	return result
}

// convertResponse converts a LangChainGo ContentResponse to AgentMesh Response.
func (m *Model) convertResponse(resp *llms.ContentResponse) (*model.Response, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, model.ErrNoResponse
	}

	choice := resp.Choices[0]

	// Build message parts and tool calls from the response
	var parts []message.Part
	var toolCalls []message.ToolCall

	// Add text content if present
	if choice.Content != "" {
		parts = append(parts, message.TextPart{Text: choice.Content})
	}

	// Add tool calls if present
	for _, tc := range choice.ToolCalls {
		if tc.FunctionCall != nil {
			parts = append(parts, message.FunctionCallPart{
				FunctionCall: &message.FunctionCall{
					ID:        tc.ID,
					Name:      tc.FunctionCall.Name,
					Arguments: tc.FunctionCall.Arguments,
				},
			})
			toolCalls = append(toolCalls, message.ToolCall{
				ID:        tc.ID,
				Name:      tc.FunctionCall.Name,
				Arguments: tc.FunctionCall.Arguments,
			})
		}
	}

	// Handle legacy FuncCall field
	if choice.FuncCall != nil && len(choice.ToolCalls) == 0 {
		callID := "call_" + choice.FuncCall.Name
		parts = append(parts, message.FunctionCallPart{
			FunctionCall: &message.FunctionCall{
				ID:        callID,
				Name:      choice.FuncCall.Name,
				Arguments: choice.FuncCall.Arguments,
			},
		})
		toolCalls = append(toolCalls, message.ToolCall{
			ID:        callID,
			Name:      choice.FuncCall.Name,
			Arguments: choice.FuncCall.Arguments,
		})
	}

	msg := message.NewAIMessage(parts)
	msg.ToolCalls = toolCalls

	return &model.Response{
		Message:      msg,
		Reasoning:    choice.ReasoningContent,
		FinishReason: choice.StopReason,
		Metadata:     choice.GenerationInfo,
		Partial:      false,
	}, nil
}
