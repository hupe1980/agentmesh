package openai

import (
	"context"
	"encoding/json"
	"fmt"
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
	Model               string
	Temperature         float64
	MaxCompletionTokens int64
}

type Model struct {
	client         Client
	model          string
	opts           Options
	tools          []tool.Tool
	responseFormat map[string]any // JSON schema for structured output
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
		Model:               openai.ChatModelGPT4oMini,
		Temperature:         0.7,
		MaxCompletionTokens: 4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	modelName := opts.Model
	if modelName == "" {
		modelName = openai.ChatModelGPT4oMini
	}

	return &Model{client: wrapper, model: modelName, opts: opts}
}

// BindTools returns a copy of the model configured with the provided tools.
func (m *Model) BindTools(tools ...tool.Tool) model.Model {
	if m == nil {
		return nil
	}

	clone := *m
	clone.opts = m.opts
	clone.tools = normalizeTools(tools)

	return &clone
}

// WithStructuredOutput returns a copy of the model configured to generate
// structured JSON output conforming to the provided schema.
func (m *Model) WithStructuredOutput(schema map[string]any) model.Model {
	if m == nil {
		return nil
	}

	clone := *m
	clone.opts = m.opts
	clone.tools = m.tools
	clone.responseFormat = schema

	return &clone
}

// Name returns the configured OpenAI model identifier.
func (m *Model) Name() string {
	return m.model
}

// Generate executes a chat completion request against the OpenAI API.
func (m *Model) Generate(ctx context.Context, msgs []message.Message) (message.Message, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("generate requires at least one message")
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Model:    m.model,
		Messages: converted,
	}

	if err := m.applyOptions(&params); err != nil {
		return nil, err
	}

	completion, err := m.client.ChatCompletions(ctx, params)
	if err != nil {
		return nil, err
	}
	if completion == nil || len(completion.Choices) == 0 {
		return nil, fmt.Errorf("openai chat completion returned no choices")
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
		for idx, call := range choice.Message.ToolCalls {
			if call.Type != "function" {
				continue
			}
			fn := call.AsFunction()
			var args map[string]any
			if fn.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(fn.Function.Arguments), &args); err != nil {
					return nil, fmt.Errorf("tool call[%d]: parse arguments: %w", idx, err)
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
		return nil, fmt.Errorf("openai chat completion returned empty message")
	}

	return aiMessage, nil
}

// Stream implements incremental streaming for chat completions.
func (m *Model) Stream(ctx context.Context, msgs []message.Message) (*model.Stream, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("stream requires at least one message")
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Model:    m.model,
		Messages: converted,
	}

	if err := m.applyOptions(&params); err != nil {
		return nil, err
	}

	streamCtx, cancel := context.WithCancel(ctx)

	apiStream := m.client.ChatCompletionsStreaming(streamCtx, params)
	if err := apiStream.Err(); err != nil {
		cancel()
		return nil, err
	}

	chunkCh := make(chan model.StreamChunk)

	go func() {
		defer close(chunkCh)
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

		flushError := func(err error) {
			chunkCh <- model.StreamChunk{Err: err, Final: true}
		}

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
				chunkCh <- model.StreamChunk{Text: delta.Content, Message: aiMessage}
			}

			if delta.Refusal != "" {
				textBuilder.WriteString(delta.Refusal)
				aiMessage := message.NewAIMessageFromText(delta.Refusal)
				chunkCh <- model.StreamChunk{Text: delta.Refusal, Message: aiMessage}
			}

			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
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
			flushError(err)
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
						flushError(fmt.Errorf("tool call[%d]: parse arguments: %w", idx, err))
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
			flushError(fmt.Errorf("openai chat completion returned empty message"))
			return
		}

		chunkCh <- model.StreamChunk{Message: aiMessage, Final: true}
	}()

	return model.NewStream(chunkCh, cancel), nil
}

func (m *Model) applyOptions(params *openai.ChatCompletionNewParams) error {
	if m == nil || params == nil {
		return nil
	}

	params.Temperature = param.NewOpt(m.opts.Temperature)
	params.MaxCompletionTokens = param.NewOpt(m.opts.MaxCompletionTokens)

	// Apply structured output if configured
	if m.responseFormat != nil {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type: "json_schema",
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "response",
					Schema: m.responseFormat,
					Strict: param.NewOpt(true),
				},
			},
		}
	}

	if len(m.tools) == 0 {
		return nil
	}

	converted, err := convertTools(m.tools)
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
