package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sort"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/shared"
)

// Stream represents a streaming response from the OpenAI API.
type Stream interface {
	Next() bool
	Current() openai.ChatCompletionChunk
	Close() error
	Err() error
}

// Client defines the interface for interacting with the OpenAI API.
type Client interface {
	ChatCompletions(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	ChatCompletionsStreaming(ctx context.Context, req openai.ChatCompletionNewParams) Stream
}

// ClientWrapper wraps an OpenAI client to implement the Client interface.
type ClientWrapper struct {
	inner *openai.Client
}

// NewClientWrapper creates a new ClientWrapper.
func NewClientWrapper(client *openai.Client) *ClientWrapper {
	return &ClientWrapper{
		inner: client,
	}
}

// ChatCompletions implements the ChatCompletions method of the Client interface.
func (c *ClientWrapper) ChatCompletions(
	ctx context.Context,
	req openai.ChatCompletionNewParams,
) (*openai.ChatCompletion, error) {
	return c.inner.Chat.Completions.New(ctx, req)
}

// ChatCompletionsStreaming implements the ChatCompletionsStreaming method of the Client interface.
func (c *ClientWrapper) ChatCompletionsStreaming(ctx context.Context, req openai.ChatCompletionNewParams) Stream {
	return c.inner.Chat.Completions.NewStreaming(ctx, req)
}

type Options struct {
	model               string
	temperature         float64
	maxCompletionTokens int64
	tools               []tool.Tool
	responseFormat      map[string]any // JSON schema for structured output
}

type Model struct {
	client Client
	model  string
	opts   Options
}

func NewModel(optFns ...func(o *Options)) *Model {
	client := openai.NewClient()
	return NewModelFromClient(&client, optFns...)
}

func NewModelFromClient(client *openai.Client, optFns ...func(o *Options)) *Model {
	return NewModelFromClientWrapper(NewClientWrapper(client), optFns...)
}

func NewModelFromClientWrapper(wrapper *ClientWrapper, optFns ...func(o *Options)) *Model {
	opts := Options{
		model:               openai.ChatModelGPT4oMini,
		temperature:         0.7,
		maxCompletionTokens: 4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	modelName := opts.model
	if modelName == "" {
		modelName = openai.ChatModelGPT4oMini
	}

	return &Model{client: wrapper, model: modelName, opts: opts}
}

// WithModel returns a new model configured to use the specified model name.
func WithModel(modelName string) func(o *Options) {
	return func(o *Options) {
		o.model = modelName
	}
}

// WithTemperature returns a new model with the specified temperature.
// Temperature controls randomness in the output (0.0 to 2.0).
func WithTemperature(temperature float64) func(o *Options) {
	return func(o *Options) {
		o.temperature = temperature
	}
}

// WithMaxCompletionTokens returns a new model with the specified maximum completion tokens.
func WithMaxCompletionTokens(maxTokens int64) func(o *Options) {
	return func(o *Options) {
		o.maxCompletionTokens = maxTokens
	}
}

// BindTools returns a new model configured with the provided tools.
func (m *Model) BindTools(tools ...tool.Tool) model.Model {
	return NewModelFromClientWrapper(m.client.(*ClientWrapper), func(o *Options) {
		*o = m.opts
		o.tools = normalizeTools(tools)
	})
}

// WithStructuredOutput returns a new model configured to generate
// structured JSON output conforming to the provided schema.
func (m *Model) WithStructuredOutput(schema map[string]any) model.Model {
	return NewModelFromClientWrapper(m.client.(*ClientWrapper), func(o *Options) {
		*o = m.opts
		o.responseFormat = schema
	})
}

// Name returns the configured OpenAI model identifier.
func (m *Model) Name() string {
	return m.model
}

// Generate executes a chat completion request against the OpenAI API.
// Returns an iterator that yields messages as they are received.
// For streaming, multiple intermediate messages are yielded followed by the final complete message.
// For non-streaming (blocking), only the final message is yielded.
//
//nolint:gocyclo // Generation requires handling many message and response types
func (m *Model) Generate(ctx context.Context, msgs []message.Message) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		if len(msgs) == 0 {
			yield(nil, fmt.Errorf("generate requires at least one message"))
			return
		}

		converted, err := convertMessagesToOpenAI(msgs)
		if err != nil {
			yield(nil, err)
			return
		}

		params := openai.ChatCompletionNewParams{
			Model:    m.model,
			Messages: converted,
		}

		if err := m.applyOptions(&params); err != nil {
			yield(nil, err)
			return
		}

		// Try streaming first
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		apiStream := m.client.ChatCompletionsStreaming(streamCtx, params)
		if apiStream.Err() == nil {
			// Streaming successful
			m.streamGenerate(apiStream, yield, cancel)
			return
		}

		// Fall back to non-streaming
		completion, err := m.client.ChatCompletions(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		if completion == nil || len(completion.Choices) == 0 {
			yield(nil, fmt.Errorf("openai chat completion returned no choices"))
			return
		}

		choice := completion.Choices[0]
		text := strings.TrimSpace(choice.Message.Content)
		if text == "" {
			text = strings.TrimSpace(choice.Message.Refusal)
		}

		var parts message.Parts
		if text != "" {
			parts = message.Parts{message.NewTextPart(text)}
		}

		aiMessage := message.NewAIMessage(parts)

		if len(choice.Message.ToolCalls) > 0 {
			toolCalls := make([]message.ToolCall, 0, len(choice.Message.ToolCalls))
			for idx := range choice.Message.ToolCalls {
				if choice.Message.ToolCalls[idx].Type != "function" {
					continue
				}
				fn := choice.Message.ToolCalls[idx].AsFunction()
				var args map[string]any
				if fn.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(fn.Function.Arguments), &args); err != nil {
						yield(nil, fmt.Errorf("tool call[%d]: parse arguments: %w", idx, err))
						return
					}
				}
				toolCalls = append(toolCalls, message.ToolCall{
					ID:        fn.ID,
					Name:      fn.Function.Name,
					Type:      string(fn.Type),
					Arguments: args,
				})
			}
			if len(toolCalls) > 0 {
				aiMessage.ToolCalls = toolCalls
			}
		}

		if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
			yield(nil, fmt.Errorf("openai chat completion returned empty message"))
			return
		}

		yield(aiMessage, nil)
	}
}

// streamGenerate handles streaming responses from OpenAI API
//
//nolint:gocyclo // Streaming requires handling many delta types and states
func (m *Model) streamGenerate(
	apiStream Stream,
	yield func(message.Message, error) bool,
	cancel context.CancelFunc,
) {
	defer cancel()
	defer func() { _ = apiStream.Close() }() // Best effort close

	type toolCallAccumulator struct {
		id        string
		typ       string
		name      strings.Builder
		arguments strings.Builder
	}

	textBuilder := &strings.Builder{}
	toolCalls := make(map[int64]*toolCallAccumulator)

	for apiStream.Next() {
		chunk := apiStream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		delta := choice.Delta

		if delta.Content != "" {
			textBuilder.WriteString(delta.Content)
			aiMessage := message.NewAIMessageFromText(delta.Content)
			if !yield(aiMessage, nil) {
				return
			}
		}

		if delta.Refusal != "" {
			textBuilder.WriteString(delta.Refusal)
			aiMessage := message.NewAIMessageFromText(delta.Refusal)
			if !yield(aiMessage, nil) {
				return
			}
		}

		if len(delta.ToolCalls) > 0 {
			for i := range delta.ToolCalls {
				tc := &delta.ToolCalls[i]
				acc, ok := toolCalls[tc.Index]
				if !ok {
					acc = &toolCallAccumulator{}
					toolCalls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Type != "" {
					acc.typ = tc.Type
				}
				if name := tc.Function.Name; name != "" {
					acc.name.WriteString(name)
				}
				if args := tc.Function.Arguments; args != "" {
					acc.arguments.WriteString(args)
				}
			}
		}
	}

	if err := apiStream.Err(); err != nil {
		yield(nil, err)
		return
	}

	finalText := strings.TrimSpace(textBuilder.String())
	var parts message.Parts
	if finalText != "" {
		parts = message.Parts{message.NewTextPart(finalText)}
	}

	aiMessage := message.NewAIMessage(parts)

	if len(toolCalls) > 0 {
		indices := make([]int, 0, len(toolCalls))
		for idx := range toolCalls {
			indices = append(indices, int(idx))
		}
		sort.Ints(indices)

		for _, idx := range indices {
			acc := toolCalls[int64(idx)]
			argsRaw := strings.TrimSpace(acc.arguments.String())
			var args map[string]any
			if argsRaw != "" {
				if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
					yield(nil, fmt.Errorf("tool call[%d]: parse arguments: %w", idx, err))
					return
				}
			}
			aiMessage.ToolCalls = append(aiMessage.ToolCalls, message.ToolCall{
				ID:        acc.id,
				Name:      acc.name.String(),
				Type:      acc.typ,
				Arguments: args,
			})
		}
	}

	if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
		yield(nil, fmt.Errorf("openai chat completion returned empty message"))
		return
	}

	yield(aiMessage, nil)
}

func (m *Model) applyOptions(params *openai.ChatCompletionNewParams) error {
	if m == nil || params == nil {
		return nil
	}

	params.Temperature = param.NewOpt(m.opts.temperature)
	params.MaxCompletionTokens = param.NewOpt(m.opts.maxCompletionTokens)

	// Apply structured output if configured
	if m.opts.responseFormat != nil {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type: "json_schema",
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "response",
					Schema: m.opts.responseFormat,
					Strict: param.NewOpt(true),
				},
			},
		}
	}

	if len(m.opts.tools) == 0 {
		return nil
	}

	converted, err := convertTools(m.opts.tools)
	if err != nil {
		return err
	}
	if len(converted) > 0 {
		params.Tools = converted
	}

	return nil
}

func convertTools(tools []tool.Tool) ([]openai.ChatCompletionToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	converted := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for idx, tool := range tools {
		if tool == nil {
			continue
		}
		definition := tool.Definition()
		if definition == nil {
			continue
		}
		if definition.Type != "" && definition.Type != "function" {
			return nil, fmt.Errorf("openai model: unsupported tool type %q", definition.Type)
		}

		fn := definition.Function
		if fn.Name == "" {
			return nil, fmt.Errorf("openai model: tool at index %d missing function name", idx)
		}

		function := shared.FunctionDefinitionParam{
			Name: fn.Name,
		}
		if fn.Description != "" {
			function.Description = param.NewOpt(fn.Description)
		}
		if len(fn.Parameters) > 0 {
			function.Parameters = shared.FunctionParameters(fn.Parameters)
		}

		converted = append(converted, openai.ChatCompletionFunctionTool(function))
	}

	return converted, nil
}

func convertMessagesToOpenAI(messages []message.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for idx, msg := range messages {
		if msg == nil {
			return nil, fmt.Errorf("messages[%d]: message must not be nil", idx)
		}

		text, err := joinTextParts(msg.Parts())
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", idx, err)
		}

		var converted openai.ChatCompletionMessageParamUnion
		switch msg.Type() {
		case message.TypeSystem:
			converted = openai.SystemMessage(text)
		case message.TypeHuman:
			converted = openai.UserMessage(text)
		case message.TypeAI:
			aiMsg, ok := msg.(*message.AIMessage)
			if !ok {
				return nil, fmt.Errorf("messages[%d]: expected *message.AIMessage for ai type", idx)
			}

			assistant := openai.ChatCompletionAssistantMessageParam{}
			if text != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(text),
				}
			}

			toolCalls, err := convertToolCalls(aiMsg.ToolCalls)
			if err != nil {
				return nil, fmt.Errorf("messages[%d]: %w", idx, err)
			}
			if len(toolCalls) > 0 {
				assistant.ToolCalls = toolCalls
			}

			converted = openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}
		case message.TypeTool:
			toolMsg, ok := msg.(*message.ToolMessage)
			if !ok {
				return nil, fmt.Errorf("messages[%d]: expected *message.ToolMessage for tool type", idx)
			}
			converted = openai.ToolMessage(text, toolMsg.ToolCallID)
		default:
			return nil, fmt.Errorf("unsupported message type %q", msg.Type())
		}
		result = append(result, converted)
	}
	return result, nil
}

func convertToolCalls(calls []message.ToolCall) ([]openai.ChatCompletionMessageToolCallUnionParam, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for idx, call := range calls {
		if call.Name == "" {
			return nil, fmt.Errorf("tool call[%d]: missing name", idx)
		}

		arguments := "{}"
		if len(call.Arguments) > 0 {
			payload, err := json.Marshal(call.Arguments)
			if err != nil {
				return nil, fmt.Errorf("tool call[%d]: marshal arguments: %w", idx, err)
			}
			arguments = string(payload)
		}

		callID := call.ID
		if callID == "" {
			callID = fmt.Sprintf("%s-%d", call.Name, idx)
		}

		toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: callID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      call.Name,
					Arguments: arguments,
				},
			},
		})
	}
	return toolCalls, nil
}

func normalizeTools(tools []tool.Tool) []tool.Tool {
	if len(tools) == 0 {
		return nil
	}

	dedup := make([]tool.Tool, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))

	for _, tool := range tools {
		if tool == nil {
			continue
		}

		name := tool.Name()
		if name != "" {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
		}

		dedup = append(dedup, tool)
	}

	if len(dedup) == 0 {
		return nil
	}

	return append([]tool.Tool(nil), dedup...)
}

func joinTextParts(parts message.Parts) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for i, part := range parts {
		switch p := part.(type) {
		case message.TextPart:
			sb.WriteString(p.Text)
		case *message.TextPart:
			if p != nil {
				sb.WriteString(p.Text)
			}
		default:
			return "", fmt.Errorf("unsupported part type %T", part)
		}
		if i < len(parts)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// Compile-time interface checks
var (
	_ model.Model            = (*Model)(nil)
	_ model.ToolAware        = (*Model)(nil)
	_ model.StructuredOutput = (*Model)(nil)
)
