// Package openai provides an implementation of model.Model using the OpenAI
// Chat Completions API (including streaming + function/tool calling). It
// adapts AgentMesh's normalized Request/Response structures into the SDK's
// message format and back.
package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
	"github.com/openai/openai-go"
)

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
	client *openai.Client
	opts   Options
}

// NewModel creates a new OpenAI model using the official client
func NewModel(optFns ...func(o *Options)) *Model {
	client := openai.NewClient()
	return NewModelFromClient(&client, optFns...)
}

// NewModelFromClient creates a new OpenAI model from an existing client
func NewModelFromClient(client *openai.Client, optFns ...func(o *Options)) *Model {
	opts := Options{
		Model:               openai.ChatModelGPT4oMini,
		Temperature:         0.7,
		MaxCompletionTokens: 4096,
	}
	for _, fn := range optFns {
		fn(&opts)
	}
	return &Model{client: client, opts: opts}
}

// Generate implements unified streaming / non-streaming generation.
// It adapts OpenAI Chat Completions (with function/tool calling) into model.Response events.
func (m *Model) Generate(ctx context.Context, req model.Request) (<-chan model.Response, <-chan error) {
	out := make(chan model.Response, 32)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		toolResponses, order := collectToolResponses(req)
		messages := buildMessages(req, toolResponses, order)
		params := m.buildParams(req, messages)
		if req.Stream {
			m.handleStreaming(ctx, params, out, errCh)
			return
		}
		m.handleNonStreaming(ctx, params, out, errCh)
	}()
	return out, errCh
}

// collectToolResponses indexes tool (function) responses by id preserving first-seen order.
func collectToolResponses(req model.Request) (map[string]string, []string) {
	responses := map[string]string{}
	order := []string{}
	for _, c := range req.Contents {
		if c.Role != "tool" {
			continue
		}
		for _, p := range c.Parts {
			fr, ok := p.(core.FunctionResponsePart)
			if !ok || fr.FunctionResponse.ID == "" {
				continue
			}
			if _, exists := responses[fr.FunctionResponse.ID]; exists {
				continue
			}
			var text string
			if s, ok := fr.FunctionResponse.Response.(string); ok {
				text = s
			} else {
				text = fmt.Sprintf("%v", fr.FunctionResponse.Response)
			}
			responses[fr.FunctionResponse.ID] = text
			order = append(order, fr.FunctionResponse.ID)
		}
	}
	return responses, order
}

// buildMessages converts normalized contents into OpenAI chat messages while
// attaching matching tool responses immediately after assistant tool calls.
func buildMessages(
	req model.Request,
	toolResponses map[string]string,
	order []string,
) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion
	for _, c := range req.Contents {
		if c.Role == "tool" {
			continue
		}
		var textBuilder strings.Builder
		for _, p := range c.Parts {
			if tp, ok := p.(core.TextPart); ok {
				textBuilder.WriteString(tp.Text)
			}
		}
		text := textBuilder.String()
		switch c.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(text))
		case "user":
			messages = append(messages, openai.UserMessage(text))
		case "assistant":
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
	for _, id := range order {
		if resp, ok := toolResponses[id]; ok {
			messages = append(messages, openai.ToolMessage(resp, id))
		}
	}
	return messages
}

// extractToolCalls extracts tool call parts and returns OpenAI formatted tool calls + ordered IDs.
func extractToolCalls(c core.Content) ([]openai.ChatCompletionMessageToolCallParam, []string) {
	var toolCalls []openai.ChatCompletionMessageToolCallParam
	var callIDs []string
	for _, p := range c.Parts {
		if fc, ok := p.(core.FunctionCallPart); ok {
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
				ID:   fc.FunctionCall.ID,
				Type: "function",
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      fc.FunctionCall.Name,
					Arguments: fc.FunctionCall.Arguments,
				},
			})
			callIDs = append(callIDs, fc.FunctionCall.ID)
		}
	}
	return toolCalls, callIDs
}

// buildParams assembles the OpenAI request parameters including tool definitions.
func (m *Model) buildParams(
	req model.Request,
	messages []openai.ChatCompletionMessageParamUnion,
) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Messages:            messages,
		Model:               m.opts.Model,
		Temperature:         openai.Float(m.opts.Temperature),
		MaxCompletionTokens: openai.Int(m.opts.MaxCompletionTokens),
	}
	if len(req.Tools) == 0 {
		return params
	}
	tools := make([]openai.ChatCompletionToolParam, len(req.Tools))
	for i, tdef := range req.Tools {
		tools[i] = openai.ChatCompletionToolParam{
			Type: "function",
			Function: openai.FunctionDefinitionParam{
				Name:        tdef.Function.Name,
				Description: openai.String(tdef.Function.Description),
				Parameters:  tdef.Function.Parameters,
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
	out chan<- model.Response,
	errCh chan<- error,
) {
	stream := m.client.Chat.Completions.NewStreaming(ctx, params)
	var textBuilder strings.Builder
	toolAgg := map[int64]*aggCall{}
	for stream.Next() {
		ck := stream.Current()
		for _, ch := range ck.Choices {
			m.emitTextDelta(ch, &textBuilder, out)
			m.emitToolCallDeltas(ch, toolAgg, out)
			if ch.FinishReason != "" {
				m.emitFinalChunk(ch, &textBuilder, toolAgg, out)
			}
		}
	}
	if err := stream.Err(); err != nil {
		errCh <- fmt.Errorf("openai streaming error: %w", err)
	}
}

func (m *Model) emitTextDelta(
	ch openai.ChatCompletionChunkChoice,
	builder *strings.Builder,
	out chan<- model.Response,
) {
	if ch.Delta.Content == "" {
		return
	}
	builder.WriteString(ch.Delta.Content)
	out <- model.Response{
		Partial: true,
		Content: core.Content{
			Role:  "assistant",
			Parts: []core.Part{core.TextPart{Text: ch.Delta.Content}},
		},
	}
}

func (m *Model) emitToolCallDeltas(
	ch openai.ChatCompletionChunkChoice,
	agg map[int64]*aggCall,
	out chan<- model.Response,
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
		out <- model.Response{
			Partial: true,
			Content: core.Content{
				Role: "assistant",
				Parts: []core.Part{core.FunctionCallPart{FunctionCall: core.FunctionCall{
					ID:        ac.id,
					Name:      ac.name,
					Arguments: ac.args,
				}}},
			},
		}
	}
}

func (m *Model) emitFinalChunk(
	ch openai.ChatCompletionChunkChoice,
	builder *strings.Builder,
	toolAgg map[int64]*aggCall,
	out chan<- model.Response,
) {
	finalParts := make([]core.Part, 0, len(toolAgg)+1)
	if builder.Len() > 0 {
		finalParts = append(finalParts, core.TextPart{Text: builder.String()})
	}
	for _, ac := range toolAgg {
		finalParts = append(finalParts, core.FunctionCallPart{FunctionCall: core.FunctionCall{
			ID:        ac.id,
			Name:      ac.name,
			Arguments: ac.args,
		}})
	}
	out <- model.Response{
		Partial:      false,
		Content:      core.Content{Role: "assistant", Parts: finalParts},
		FinishReason: ch.FinishReason,
	}
}

// handleNonStreaming processes a normal (non-streaming) completion.
func (m *Model) handleNonStreaming(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	out chan<- model.Response,
	errCh chan<- error,
) {
	resp, err := m.client.Chat.Completions.New(ctx, params)
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
		parts = append(parts, core.TextPart{Text: ch0.Message.Content})
	}
	for _, tc := range ch0.Message.ToolCalls {
		parts = append(parts, core.FunctionCallPart{FunctionCall: core.FunctionCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}})
	}
	out <- model.Response{
		Partial:      false,
		Content:      core.Content{Role: "assistant", Parts: parts},
		FinishReason: ch0.FinishReason,
	}
}

// Info returns metadata describing this OpenAI model implementation.
func (m *Model) Info() model.Info {
	return model.Info{
		Name:          m.opts.Model,
		Provider:      "openai",
		SupportsTools: true,
	}
}
