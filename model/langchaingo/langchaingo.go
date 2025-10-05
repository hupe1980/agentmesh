package langchaingo

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/tmc/langchaingo/llms"
)

// Model adapts a langchaingo llms.Model to agentmesh's core.Model.
type Model struct {
	llm llms.Model
}

// NewModel wraps the provided langchaingo llms.Model so it satisfies core.Model.
func NewModel(llm llms.Model) (*Model, error) {
	return &Model{
		llm: llm,
	}, nil
}

// Generate runs a completion using langchaingo's chat interface (GenerateContent).
// It builds chat messages from instructions + full message history and converts
// them to MessageContent. If req.Stream is true, partial text deltas are sent on
// the response channel as they arrive, and a final chunk is emitted upon completion.
// In non-streaming mode, a single final response is returned.
func (m *Model) Generate(
	ctx context.Context,
	req *core.ModelRequest,
) (<-chan *core.ModelResponse, <-chan error) {
	out := make(chan *core.ModelResponse, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		if req == nil {
			errCh <- fmt.Errorf("nil model request")
			return
		}

		messages := buildMessageContents(req)

		if len(messages) == 0 {
			errCh <- fmt.Errorf("empty prompt")
			return
		}

		// Prepare call options (tools, streaming, etc.).
		var callOpts []llms.CallOption
		if len(req.Tools) > 0 {
			callOpts = append(callOpts, llms.WithTools(toLLMSTools(req.Tools)))
		}

		// Streaming mode: emit partial deltas via WithStreamingFunc and a final chunk at the end.
		if req.Stream {
			resp, err := m.llm.GenerateContent(
				ctx,
				messages,
				append(callOpts, llms.WithStreamingFunc(func(cctx context.Context, chunk []byte) error {
					// Respect cancellation
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}

					s := string(chunk)
					if s == "" {
						return nil
					}

					out <- &core.ModelResponse{
						Partial: true,
						Parts:   []core.Part{core.NewPartFromText(s)},
					}

					return nil
				}))...,
			)
			if err != nil {
				// If context canceled, surface as error channel termination
				errCh <- fmt.Errorf("langchaingo stream: %w", err)
				return
			}

			// Some providers may not invoke the streaming callback; fallback to response content.
			out <- &core.ModelResponse{
				Partial:      false,
				Parts:        buildPartsFromResponse(resp),
				FinishReason: firstStopReason(resp),
			}
			return
		}

		// Non-streaming path
		resp, err := m.llm.GenerateContent(ctx, messages, callOpts...)
		if err != nil {
			errCh <- fmt.Errorf("langchaingo generate: %w", err)
			return
		}

		out <- &core.ModelResponse{
			Partial:      false,
			Parts:        buildPartsFromResponse(resp),
			FinishReason: firstStopReason(resp),
		}
	}()

	return out, errCh
}

// Capabilities reports the supported feature set of the wrapped LangChainGo model.
// The generic llms.Model interface does not currently expose native structured
// output, so we return the zero-value capabilities (all false).
func (m *Model) Capabilities() *core.ModelCapabilities {
	return &core.ModelCapabilities{SupportsStructuredOutput: false}
}

// buildChatMessages converts instructions and message history into llms.ChatMessage values.
func buildMessageContents(req *core.ModelRequest) []llms.MessageContent {
	var out []llms.MessageContent
	if req.Instructions != "" {
		out = append(out, llms.TextParts(llms.ChatMessageTypeSystem, req.Instructions))
	}

	toolResponses := collectToolResponses(req)

	for _, msg := range req.Messages {
		if msg == nil {
			continue
		}

		// Aggregate text parts
		var tb strings.Builder
		for _, p := range msg.Parts {
			if tp, ok := p.(*core.TextPart); ok {
				tb.WriteString(tp.Text)
			}
		}

		text := tb.String()

		switch msg.Role {
		case core.RoleSystem:
			if text != "" {
				out = append(out, llms.TextParts(llms.ChatMessageTypeSystem, text))
			}
		case core.RoleUser:
			if text != "" {
				out = append(out, llms.TextParts(llms.ChatMessageTypeHuman, text))
			}
		case core.RoleTool:
			// Forward text-only tool messages; function responses are attached after assistant calls.
			if text != "" {
				out = append(out, llms.TextParts(llms.ChatMessageTypeTool, text))
			}
		case core.RoleAssistant:
			// Build assistant message parts: optional text + tool calls
			parts := make([]llms.ContentPart, 0, len(msg.Parts)+1)
			if text != "" {
				parts = append(parts, llms.TextPart(text))
			}

			// Gather tool call IDs and names in order
			var callIDs []string
			callNames := map[string]string{}
			idx := 0

			for _, p := range msg.Parts {
				if fc, ok := p.(*core.FunctionCallPart); ok && fc.FunctionCall != nil && fc.FunctionCall.Name != "" {
					id := fc.FunctionCall.ID
					if id == "" {
						idx++
						id = fmt.Sprintf("tc-%d", idx)
					}
					callIDs = append(callIDs, id)
					callNames[id] = fc.FunctionCall.Name
					parts = append(parts, llms.ToolCall{
						ID:   id,
						Type: "function",
						FunctionCall: &llms.FunctionCall{
							Name:      fc.FunctionCall.Name,
							Arguments: fc.FunctionCall.Arguments,
						},
					})
				}
			}

			out = append(out, llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: parts})

			// Attach tool responses right after
			for _, id := range callIDs {
				if id == "" {
					continue
				}
				if resp, ok := toolResponses[id]; ok {
					out = append(out, llms.MessageContent{
						Role: llms.ChatMessageTypeTool,
						Parts: []llms.ContentPart{
							llms.ToolCallResponse{ToolCallID: id, Name: callNames[id], Content: resp},
						},
					})
					delete(toolResponses, id)
				}
			}
		default:
			if text != "" {
				out = append(out, llms.TextParts(llms.ChatMessageTypeGeneric, text))
			}
		}
	}

	return out
}

// collectToolResponses indexes tool (function) responses by id preserving first-seen order.
func collectToolResponses(req *core.ModelRequest) map[string]string {
	responses := map[string]string{}
	for _, c := range req.Messages {
		if c == nil || c.Role != core.RoleTool {
			continue
		}
		for _, p := range c.Parts {
			fr, ok := p.(*core.FunctionResponsePart)
			if !ok || fr.FunctionResponse == nil || fr.FunctionResponse.ID == "" {
				continue
			}

			// Preserve first-seen response for each id.
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
		}
	}

	return responses
}

// toLLMSTools converts core.ToolDefinition into langchaingo llms.Tool slice.
func toLLMSTools(defs []core.ToolDefinition) []llms.Tool {
	if len(defs) == 0 {
		return nil
	}

	tools := make([]llms.Tool, 0, len(defs))
	for _, d := range defs {
		fn := d.Function
		tools = append(tools, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        fn.Name,
				Description: fn.Description,
				Parameters:  fn.Parameters,
			},
		})
	}

	return tools
}

// firstContent extracts the first choice content from a ContentResponse (safe default "").
func firstContent(resp *llms.ContentResponse) string {
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return ""
	}

	return resp.Choices[0].Content
}

// firstStopReason extracts the first choice stop reason or returns "stop".
func firstStopReason(resp *llms.ContentResponse) string {
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil || resp.Choices[0].StopReason == "" {
		return "stop"
	}

	return resp.Choices[0].StopReason
}

// buildPartsFromResponse constructs core.Parts from a langchaingo ContentResponse.
// It includes a text part (using defaultText if provided, otherwise the response content),
// followed by any tool/function call parts from the first choice.
func buildPartsFromResponse(resp *llms.ContentResponse) []core.Part {
	parts := make([]core.Part, 0, 4)

	// Text content
	text := firstContent(resp)
	if text != "" {
		parts = append(parts, core.NewPartFromText(text))
	}

	// Tool calls (first choice only)
	if resp != nil && len(resp.Choices) > 0 && resp.Choices[0] != nil {
		ch := resp.Choices[0]
		// Prefer explicit ToolCalls if present; else FuncCall; else fall back to GenerationInfo
		if len(ch.ToolCalls) > 0 {
			for i, tc := range ch.ToolCalls {
				if tc.FunctionCall != nil && tc.FunctionCall.Name != "" {
					id := tc.ID
					if id == "" {
						id = fmt.Sprintf("tc-%d", i+1)
					}

					parts = append(parts, core.NewPartFromFunctionCall(id, tc.FunctionCall.Name, tc.FunctionCall.Arguments))
				}
			}
		}
	}

	return parts
}
