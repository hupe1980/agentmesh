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

		chatMsgs := buildChatMessages(req)
		messages := toMessageContent(chatMsgs)

		if len(messages) == 0 {
			errCh <- fmt.Errorf("empty prompt")
			return
		}

		// Streaming mode: emit partial deltas via WithStreamingFunc and a final chunk at the end.
		if req.Stream {
			var builder strings.Builder
			resp, err := m.llm.GenerateContent(
				ctx,
				messages,
				llms.WithStreamingFunc(func(cctx context.Context, chunk []byte) error {
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
					builder.WriteString(s)
					out <- &core.ModelResponse{
						Partial: true,
						Parts:   []core.Part{core.NewPartFromText(s)},
					}
					return nil
				}),
			)
			if err != nil {
				// If context canceled, surface as error channel termination
				errCh <- fmt.Errorf("langchaingo stream: %w", err)
				return
			}

			out <- &core.ModelResponse{
				Partial:      false,
				Parts:        []core.Part{core.NewPartFromText(builder.String())},
				FinishReason: firstStopReason(resp),
			}
			return
		}

		// Non-streaming path
		resp, err := m.llm.GenerateContent(ctx, messages)
		if err != nil {
			errCh <- fmt.Errorf("langchaingo generate: %w", err)
			return
		}

		out <- &core.ModelResponse{
			Partial:      false,
			Parts:        []core.Part{core.NewPartFromText(firstContent(resp))},
			FinishReason: firstStopReason(resp),
		}
	}()

	return out, errCh
}

// buildChatMessages converts instructions and message history into llms.ChatMessage values.
// Role mapping: system -> SystemChatMessage, user -> HumanChatMessage, assistant ->
// AIChatMessage, tool -> ToolChatMessage (ID omitted), others -> GenericChatMessage.
// Only text parts are included; function/tool-call parts are ignored.
func buildChatMessages(req *core.ModelRequest) []llms.ChatMessage {
	var out []llms.ChatMessage
	if req.Instructions != "" {
		out = append(out, llms.SystemChatMessage{Content: req.Instructions})
	}

	for _, msg := range req.Messages {
		if msg == nil {
			continue
		}

		var tb strings.Builder
		for _, p := range msg.Parts {
			if tp, ok := p.(*core.TextPart); ok {
				tb.WriteString(tp.Text)
			}
		}

		text := strings.TrimSpace(tb.String())
		if text == "" {
			continue
		}

		switch msg.Role {
		case core.RoleSystem:
			out = append(out, llms.SystemChatMessage{Content: text})
		case core.RoleUser:
			out = append(out, llms.HumanChatMessage{Content: text})
		case core.RoleAssistant:
			out = append(out, llms.AIChatMessage{Content: text})
		case core.RoleTool:
			// ToolChatMessage requires an ID; it's optional for our text-only path.
			out = append(out, llms.ToolChatMessage{ID: "", Content: text})
		default:
			out = append(out, llms.GenericChatMessage{Content: text, Role: string(msg.Role)})
		}
	}

	return out
}

// toMessageContent flattens chat messages into text-only MessageContent items for GenerateContent.
func toMessageContent(msgs []llms.ChatMessage) []llms.MessageContent {
	out := make([]llms.MessageContent, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}

		out = append(out, llms.TextParts(m.GetType(), m.GetContent()))
	}

	return out
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
