// Package ollama provides an adapter for integrating Ollama models with AgentMesh.
// It enables local model execution without API keys or cloud dependencies.
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/ollama/ollama/api"
)

// Client defines the interface for interacting with the Ollama API.
type Client interface {
	Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error
	Generate(ctx context.Context, req *api.GenerateRequest, fn api.GenerateResponseFunc) error
}

// ClientWrapper wraps an Ollama client to implement the Client interface.
type ClientWrapper struct {
	inner *api.Client
}

// NewClientWrapper creates a new ClientWrapper.
// Returns an error if the client parameter is nil.
func NewClientWrapper(client *api.Client) (*ClientWrapper, error) {
	if err := validate.NotNil(client, "ollama: client"); err != nil {
		return nil, err
	}

	return &ClientWrapper{
		inner: client,
	}, nil
}

// Chat implements the Chat method of the Client interface.
func (c *ClientWrapper) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	return c.inner.Chat(ctx, req, fn)
}

// Generate implements the Generate method of the Client interface.
func (c *ClientWrapper) Generate(ctx context.Context, req *api.GenerateRequest, fn api.GenerateResponseFunc) error {
	return c.inner.Generate(ctx, req, fn)
}

// Options configures Ollama model behavior.
type Options struct {
	model       string
	temperature float64
	numPredict  int
	topK        int
	topP        float64
	seed        int
}

// Model wraps the Ollama API client for chat completion.
type Model struct {
	client Client
	model  string
	opts   Options
}

// NewModel creates a new Ollama model using the default client from environment.
func NewModel(optFns ...func(o *Options)) *Model {
	client, _ := api.ClientFromEnvironment()
	model, _ := NewModelFromClient(client, optFns...)
	return model
}

// NewModelFromClient creates a model from an existing Ollama client.
// Returns an error if the client is nil.
func NewModelFromClient(client *api.Client, optFns ...func(o *Options)) (*Model, error) {
	wrapper, err := NewClientWrapper(client)
	if err != nil {
		return nil, err
	}

	return NewModelFromClientWrapper(wrapper, optFns...)
}

// NewModelFromClientWrapper creates a model from a wrapped client.
// Returns an error if the wrapper is nil.
func NewModelFromClientWrapper(wrapper *ClientWrapper, optFns ...func(o *Options)) (*Model, error) {
	if err := validate.NotNil(wrapper, "ollama: wrapper"); err != nil {
		return nil, err
	}

	opts := Options{
		model:       "llama3.2",
		temperature: 0.7,
		numPredict:  -1, // -1 means no limit
		topK:        40,
		topP:        0.9,
		seed:        -1, // -1 means random seed
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	modelName := opts.model
	if modelName == "" {
		modelName = "llama3.2"
	}

	return &Model{client: wrapper, model: modelName, opts: opts}, nil
}

// WithModel sets the Ollama model to use (e.g., "llama3.2", "mistral", "codellama").
func WithModel(modelName string) func(o *Options) {
	return func(o *Options) {
		o.model = modelName
	}
}

// WithTemperature controls randomness in the output (0.0 to 2.0).
// Lower values make output more deterministic.
func WithTemperature(temperature float64) func(o *Options) {
	return func(o *Options) {
		o.temperature = temperature
	}
}

// WithNumPredict sets the maximum number of tokens to predict.
// -1 means no limit (default).
func WithNumPredict(numPredict int) func(o *Options) {
	return func(o *Options) {
		o.numPredict = numPredict
	}
}

// WithTopK limits sampling to the K most likely next tokens.
func WithTopK(topK int) func(o *Options) {
	return func(o *Options) {
		o.topK = topK
	}
}

// WithTopP uses nucleus sampling: only tokens with cumulative probability <= P are considered.
func WithTopP(topP float64) func(o *Options) {
	return func(o *Options) {
		o.topP = topP
	}
}

// WithSeed sets the random seed for deterministic output.
// -1 means random seed (default).
func WithSeed(seed int) func(o *Options) {
	return func(o *Options) {
		o.seed = seed
	}
}

// Name returns the configured Ollama model identifier.
func (m *Model) Name() string {
	return m.model
}

// Capabilities returns the features and limitations of Ollama models.
func (m *Model) Capabilities() model.Capabilities {
	return model.Capabilities{
		Streaming:        true,
		Tools:            true, // Ollama supports tool calling
		StructuredOutput: false,
		NativeReasoning:  false,
	}
}

// Generate sends messages to the Ollama model and yields responses.
// Supports both streaming and non-streaming modes.
func (m *Model) Generate(ctx context.Context, req model.Request) iter.Seq2[model.Response, error] {
	return func(yield func(model.Response, error) bool) {
		// Convert messages to Ollama format
		messages, err := m.convertMessages(req.Messages, req.Instructions)
		if err != nil {
			yield(model.Response{}, fmt.Errorf("ollama: failed to convert messages: %w", err))
			return
		}

		// Build Ollama request
		chatReq := &api.ChatRequest{
			Model:    m.model,
			Messages: messages,
			Stream:   &req.Stream,
			Options: map[string]interface{}{
				"temperature": m.opts.temperature,
				"num_predict": m.opts.numPredict,
				"top_k":       m.opts.topK,
				"top_p":       m.opts.topP,
			},
		}

		if m.opts.seed >= 0 {
			chatReq.Options["seed"] = m.opts.seed
		}

		// Add tools if provided
		if len(req.Tools) > 0 {
			chatReq.Tools = m.convertTools(req.Tools)
		}

		// Handle streaming vs non-streaming
		if req.Stream {
			m.generateStreaming(ctx, chatReq, yield)
		} else {
			m.generateNonStreaming(ctx, chatReq, yield)
		}
	}
}

// generateStreaming handles streaming responses.
func (m *Model) generateStreaming(ctx context.Context, req *api.ChatRequest, yield func(model.Response, error) bool) {
	var fullContent strings.Builder
	var toolCalls []message.ToolCall
	var finishReason string

	err := m.client.Chat(ctx, req, func(resp api.ChatResponse) error {
		// Check for tool calls in the response
		if len(resp.Message.ToolCalls) > 0 {
			for _, tc := range resp.Message.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				toolCalls = append(toolCalls, message.ToolCall{
					ID:        tc.Function.Name, // Ollama doesn't provide separate ID
					Name:      tc.Function.Name,
					Type:      "function",
					Arguments: string(argsJSON),
				})
			}
		}

		// Accumulate text content
		if resp.Message.Content != "" {
			fullContent.WriteString(resp.Message.Content)
		}

		// Check if this is the final chunk
		if !resp.Done {
			// Yield partial response
			response := model.Response{
				Message:      m.createMessage(resp.Message.Content, nil),
				FinishReason: "",
				Partial:      true,
			}
			if !yield(response, nil) {
				return ErrYieldFalse
			}
			return nil
		}

		// Final response
		finishReason = "stop"
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		}

		response := model.Response{
			Message: m.createMessage(fullContent.String(), toolCalls),
			Usage: &model.UsageInfo{
				PromptTokens:     resp.PromptEvalCount,
				CompletionTokens: resp.EvalCount,
				ReasoningTokens:  0,
				TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
			},
			FinishReason: finishReason,
			Partial:      false,
		}

		if !yield(response, nil) {
			return ErrYieldFalse
		}
		return nil
	})

	if err != nil {
		yield(model.Response{}, fmt.Errorf("ollama: streaming failed: %w", err))
	}
}

// generateNonStreaming handles non-streaming responses.
func (m *Model) generateNonStreaming(ctx context.Context, req *api.ChatRequest, yield func(model.Response, error) bool) {
	var finalResponse api.ChatResponse

	err := m.client.Chat(ctx, req, func(resp api.ChatResponse) error {
		finalResponse = resp
		return nil
	})

	if err != nil {
		yield(model.Response{}, fmt.Errorf("ollama: generation failed: %w", err))
		return
	}

	// Convert tool calls
	var toolCalls []message.ToolCall
	if len(finalResponse.Message.ToolCalls) > 0 {
		for _, tc := range finalResponse.Message.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			toolCalls = append(toolCalls, message.ToolCall{
				ID:        tc.Function.Name,
				Name:      tc.Function.Name,
				Type:      "function",
				Arguments: string(argsJSON),
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	response := model.Response{
		Message: m.createMessage(finalResponse.Message.Content, toolCalls),
		Usage: &model.UsageInfo{
			PromptTokens:     finalResponse.PromptEvalCount,
			CompletionTokens: finalResponse.EvalCount,
			ReasoningTokens:  0,
			TotalTokens:      finalResponse.PromptEvalCount + finalResponse.EvalCount,
		},
		FinishReason: finishReason,
		Partial:      false,
	}

	yield(response, nil)
}

// convertMessages converts agentmesh messages to Ollama format.
func (m *Model) convertMessages(messages []message.Message, instructions string) ([]api.Message, error) {
	result := make([]api.Message, 0, len(messages)+1)

	// Add instructions as system message if provided
	if instructions != "" {
		result = append(result, api.Message{
			Role:    "system",
			Content: instructions,
		})
	}

	// Convert each message
	for _, msg := range messages {
		ollamaMsg, err := m.convertMessage(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, ollamaMsg)
	}

	return result, nil
}

// convertMessage converts a single agentmesh message to Ollama format.
func (m *Model) convertMessage(msg message.Message) (api.Message, error) {
	switch msg.Type() {
	case message.TypeSystem:
		return api.Message{
			Role:    "system",
			Content: m.extractTextFromParts(msg.Parts()),
		}, nil

	case message.TypeHuman:
		return api.Message{
			Role:    "user",
			Content: m.extractTextFromParts(msg.Parts()),
		}, nil

	case message.TypeAI:
		content := m.extractTextFromParts(msg.Parts())
		ollamaMsg := api.Message{
			Role:    "assistant",
			Content: content,
		}

		// Handle tool calls
		if aiMsg, ok := msg.(*message.AIMessage); ok && len(aiMsg.ToolCalls) > 0 {
			var toolCalls []api.ToolCall
			for _, tc := range aiMsg.ToolCalls {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					return api.Message{}, fmt.Errorf("ollama: invalid tool arguments: %w", err)
				}

				toolCalls = append(toolCalls, api.ToolCall{
					Function: api.ToolCallFunction{
						Name:      tc.Name,
						Arguments: args,
					},
				})
			}
			ollamaMsg.ToolCalls = toolCalls
		}

		return ollamaMsg, nil

	case message.TypeTool:
		if toolMsg, ok := msg.(*message.ToolMessage); ok {
			// Extract text from tool message parts
			var content strings.Builder
			for _, part := range toolMsg.Parts() {
				if textPart, ok := part.(*message.TextPart); ok {
					content.WriteString(textPart.Text)
				}
			}
			return api.Message{
				Role:    "tool",
				Content: content.String(),
			}, nil
		}
		return api.Message{}, ErrInvalidToolMessage

	default:
		return api.Message{}, fmt.Errorf("ollama: unsupported message type: %s", msg.Type())
	}
}

// extractTextFromParts extracts text content from message parts.
func (m *Model) extractTextFromParts(parts message.Parts) string {
	var result strings.Builder
	for _, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			result.WriteString(textPart.Text)
		}
	}
	return result.String()
}

// convertTools converts agentmesh tools to Ollama format.
func (m *Model) convertTools(tools []tool.Tool) []api.Tool {
	result := make([]api.Tool, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()
		if def == nil {
			continue
		}

		// Get parameters from function definition
		parameters := def.Function.Parameters
		if parameters == nil {
			parameters = make(map[string]any)
		}

		// Convert parameters to ToolFunctionParameters
		toolParams := api.ToolFunctionParameters{
			Type: "object",
		}

		// Extract and convert properties
		props, ok := parameters["properties"].(map[string]any)
		if !ok {
			props = make(map[string]any)
		}

		toolProps := make(map[string]api.ToolProperty, len(props))
		for k, v := range props {
			propMap, ok := v.(map[string]any)
			if !ok {
				continue
			}

			prop := api.ToolProperty{}
			if t, ok := propMap["type"].(string); ok {
				prop.Type = api.PropertyType{t}
			}
			if d, ok := propMap["description"].(string); ok {
				prop.Description = d
			}
			toolProps[k] = prop
		}
		toolParams.Properties = toolProps
		switch req := parameters["required"].(type) {
		case []string:
			toolParams.Required = req
		case []any:
			toolParams.Required = make([]string, 0, len(req))
			for _, r := range req {
				if rs, ok := r.(string); ok {
					toolParams.Required = append(toolParams.Required, rs)
				}
			}
		}

		// Build Ollama tool
		ollamaTool := api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  toolParams,
			},
		}

		result = append(result, ollamaTool)
	}

	return result
}

// createMessage creates an AI message from content and tool calls.
func (m *Model) createMessage(content string, toolCalls []message.ToolCall) message.Message {
	if len(toolCalls) > 0 {
		msg := message.NewAIMessage(message.Parts{message.NewTextPart(content)})
		msg.ToolCalls = toolCalls
		return msg
	}
	return message.NewAIMessageFromText(content)
}
