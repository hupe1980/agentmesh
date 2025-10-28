package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/openai/openai-go/v2"
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

// aggCall aggregates partial tool call streaming deltas (id, name, arguments)
// allowing reconstruction of complete function call parts when finish reason
// is emitted. Internal helper (unexported).
type aggCall struct{ id, name, args string }

// Options configure the OpenAI model adapter.
// Fields mirror a subset of Chat Completion parameters intentionally kept
// minimal; extend via functional options without breaking callers.
type Options struct {
	Model               string
	Temperature         float64
	MaxCompletionTokens int64
}

// Model wraps the OpenAI Chat Completions API behind the generic model.Model interface.
type Model struct {
	client Client
	opts   Options
}

// NewModel creates a new OpenAI model using the official client
func NewModel(optFns ...func(o *Options)) *Model {
	client := openai.NewClient()
	return NewModelFromClient(&client, optFns...)
}

// NewModelFromClient creates a new OpenAI model from an existing client
func NewModelFromClient(client *openai.Client, optFns ...func(o *Options)) *Model {
	return NewModelFromClientWrapper(NewClientWrapper(client), optFns...)
}

// NewModelFromClientWrapper creates a new OpenAI model from a ClientWrapper.
func NewModelFromClientWrapper(wrapper *ClientWrapper, optFns ...func(o *Options)) *Model {
	opts := Options{
		Model:               openai.ChatModelGPT4oMini,
		Temperature:         0.7,
		MaxCompletionTokens: 4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Model{
		client: wrapper,
		opts:   opts,
	}
}

// Generate implements unified streaming / non-streaming generation.
// It adapts OpenAI Chat Completions (with function/tool calling) into core.ModelResponse events.
func (m *Model) Generate(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
	out := make(chan *core.ModelResponse, 32)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		if req == nil {
			errCh <- fmt.Errorf("nil model request")
			return
		}

		messages := buildMessages(*req)

		params := m.buildParams(*req, messages)

		if req.Stream {
			m.handleStreaming(ctx, params, out, errCh)
			return
		}

		m.handleNonStreaming(ctx, params, out, errCh)
	}()

	return out, errCh
}

// Capabilities reports the OpenAI chat model features exposed via this adapter.
// The official API supports native structured output through response_format
// when using compatible models (e.g., gpt-4o-mini), so we surface that here.
func (m *Model) Capabilities() *core.ModelCapabilities {
	return &core.ModelCapabilities{SupportsStructuredOutput: true}
}

// collectToolResponses indexes tool (function) responses by id preserving first-seen order.
func collectToolResponses(req core.ModelRequest) map[string]string {
	responses := map[string]string{}
	for _, c := range req.Messages {
		if c.Role != core.RoleTool {
			continue
		}
		for _, p := range c.Parts {
			fr, ok := p.(*core.FunctionResponsePart)
			if !ok || fr.FunctionResponse.ID == "" {
				continue
			}

			var text string
			if s, ok := fr.FunctionResponse.Response.(string); ok {
				text = s
			} else {
				text = fmt.Sprintf("%v", fr.FunctionResponse.Response)
			}

			responses[fr.FunctionResponse.ID] = text
		}
	}

	return responses
}

// buildMessages converts normalized contents into OpenAI chat messages while
// attaching matching tool responses immediately after assistant tool calls.
func buildMessages(req core.ModelRequest) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion

	if req.Instructions != "" {
		messages = append(messages, openai.SystemMessage(req.Instructions))
	}

	toolResponses := collectToolResponses(req)

	for _, c := range req.Messages {
		if c.Role == core.RoleTool {
			// Skip here; responses will be attached after assistant tool_calls as required by API.
			continue
		}

		var textBuilder strings.Builder
		for _, p := range c.Parts {
			if tp, ok := p.(*core.TextPart); ok {
				textBuilder.WriteString(tp.Text)
			}
		}

		text := textBuilder.String()

		switch c.Role {
		case core.RoleSystem:
			messages = append(messages, openai.SystemMessage(text))
		case core.RoleUser:
			messages = append(messages, openai.UserMessage(text))
		case core.RoleAssistant:
			toolCalls, callIDs := extractToolCalls(c)
			if len(toolCalls) == 0 {
				messages = append(messages, openai.AssistantMessage(text))
				continue
			}

			messages = append(
				messages,
				openai.ChatCompletionMessageParamUnion{OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Role:      "assistant",
					ToolCalls: toolCalls,
				}},
			)

			// Immediately attach tool responses for each call id if present.
			for _, id := range callIDs {
				if id == "" {
					continue
				}

				if resp, ok := toolResponses[id]; ok {
					messages = append(messages, openai.ToolMessage(resp, id))
					delete(toolResponses, id)
				}
			}
		default:
			if text != "" {
				messages = append(messages, openai.UserMessage(text))
			}
		}
	}

	return messages
}

// extractToolCalls extracts tool call parts and returns OpenAI formatted tool calls + ordered IDs.
func extractToolCalls(c *core.Message) ([]openai.ChatCompletionMessageToolCallUnionParam, []string) {
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	var callIDs []string
	for _, p := range c.Parts {
		if fc, ok := p.(*core.FunctionCallPart); ok {
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: fc.FunctionCall.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      fc.FunctionCall.Name,
						Arguments: fc.FunctionCall.Arguments,
					},
				},
			})
			callIDs = append(callIDs, fc.FunctionCall.ID)
		}
	}
	return toolCalls, callIDs
}

// buildParams assembles the OpenAI request parameters including tool definitions.
func (m *Model) buildParams(
	req core.ModelRequest,
	messages []openai.ChatCompletionMessageParamUnion,
) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Messages:            messages,
		Model:               m.opts.Model,
		Temperature:         openai.Float(m.opts.Temperature),
		MaxCompletionTokens: openai.Int(m.opts.MaxCompletionTokens),
	}

	if os, ok := req.OutputSchema.Get(); ok {
		schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:        os.Name,
			Description: openai.String(os.Description.Or("response format for the model")),
			Schema:      os.Schema,
			Strict:      openai.Bool(os.Strict.Or(false)),
		}

		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{JSONSchema: schemaParam},
		}
	}

	if len(req.Tools) == 0 {
		return params
	}

	tools := make([]openai.ChatCompletionToolUnionParam, len(req.Tools))
	for i, tdef := range req.Tools {
		tools[i] = openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        tdef.Function.Name,
					Description: openai.String(tdef.Function.Description),
					Parameters:  tdef.Function.Parameters,
				},
			},
		}
	}
	params.Tools = tools

	return params
}

// handleStreaming processes streaming responses and forwards partial / final events.
func (m *Model) handleStreaming(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	out chan<- *core.ModelResponse,
	errCh chan<- error,
) {
	stream := m.client.ChatCompletionsStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var textBuilder strings.Builder
	toolAgg := map[int64]*aggCall{}
	sentFinal := false

	for stream.Next() {
		ck := stream.Current()
		for _, ch := range ck.Choices {
			m.emitTextDelta(ch, &textBuilder, out)
			m.emitToolCallDeltas(ch, toolAgg, out)

			// On finish reason, emit final aggregated chunk (text + tool calls).
			if ch.FinishReason != "" && !sentFinal {
				sentFinal = true
				m.emitFinalChunk(ch, &textBuilder, toolAgg, out)
			}
		}
	}

	if err := stream.Err(); err != nil {
		errCh <- fmt.Errorf("openai streaming error: %w", err)
	}
}

// emitTextDelta sends a delta text update to the output channel.
func (m *Model) emitTextDelta(
	ch openai.ChatCompletionChunkChoice,
	builder *strings.Builder,
	out chan<- *core.ModelResponse,
) {
	if ch.Delta.Content == "" {
		return
	}

	builder.WriteString(ch.Delta.Content)

	out <- &core.ModelResponse{
		Partial: true,
		Parts:   []core.Part{core.NewPartFromText(ch.Delta.Content)},
	}
}

// emitToolCallDeltas sends tool call updates to the output channel.
func (m *Model) emitToolCallDeltas(
	ch openai.ChatCompletionChunkChoice,
	agg map[int64]*aggCall,
	out chan<- *core.ModelResponse,
) {
	for _, tc := range ch.Delta.ToolCalls {
		ac, ok := agg[tc.Index]
		if !ok {
			ac = &aggCall{}
			agg[tc.Index] = ac
		}

		if tc.ID != "" {
			ac.id = tc.ID
		}

		if tc.Function.Name != "" {
			ac.name = tc.Function.Name
		}

		if tc.Function.Arguments != "" {
			ac.args += tc.Function.Arguments
		}

		out <- &core.ModelResponse{
			Partial: true,
			Parts:   []core.Part{core.NewPartFromFunctionCall(ac.id, ac.name, ac.args)},
		}
	}
}

// emitFinalChunk sends the final aggregated chunk (text + tool calls) to output.
func (m *Model) emitFinalChunk(
	ch openai.ChatCompletionChunkChoice,
	builder *strings.Builder,
	toolAgg map[int64]*aggCall,
	out chan<- *core.ModelResponse,
) {
	finalParts := make([]core.Part, 0, len(toolAgg)+1)
	if builder.Len() > 0 {
		finalParts = append(finalParts, core.NewPartFromText(builder.String()))
	}

	for _, ac := range toolAgg {
		finalParts = append(finalParts, core.NewPartFromFunctionCall(ac.id, ac.name, ac.args))
	}

	out <- &core.ModelResponse{
		Partial:      false,
		Parts:        finalParts,
		FinishReason: ch.FinishReason,
	}
}

// handleNonStreaming processes a normal (non-streaming) completion.
func (m *Model) handleNonStreaming(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	out chan<- *core.ModelResponse,
	errCh chan<- error,
) {
	resp, err := m.client.ChatCompletions(ctx, params)
	if err != nil {
		errCh <- fmt.Errorf("openai api error: %w", err)
		return
	}

	if len(resp.Choices) == 0 {
		errCh <- fmt.Errorf("no choices returned")
		return
	}

	ch0 := resp.Choices[0]

	parts := make([]core.Part, 0, len(ch0.Message.ToolCalls)+1)
	if ch0.Message.Content != "" {
		parts = append(parts, core.NewPartFromText(ch0.Message.Content))
	}

	for _, tc := range ch0.Message.ToolCalls {
		f := tc.AsFunction()
		if f.ID != "" || f.Function.Name != "" || f.Function.Arguments != "" {
			parts = append(parts, core.NewPartFromFunctionCall(f.ID, f.Function.Name, f.Function.Arguments))
		}
	}

	out <- &core.ModelResponse{
		Partial:      false,
		Parts:        parts,
		FinishReason: ch0.FinishReason,
	}
}
