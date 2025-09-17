package flow

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
)

const userAuthor = "user"

// MessagesProcessor selects and injects conversation messages into the request.
type MessagesProcessor struct{}

// NewMessagesProcessor creates a new messages processor.
func NewMessagesProcessor() *MessagesProcessor { return &MessagesProcessor{} }

// Name returns the processor's identifier.
func (p *MessagesProcessor) Name() string { return "messages" }

// belongsToBranch checks if an event belongs to a specific branch.
func belongsToBranch(reqCtxBranch string, eventBranch core.Opt[string]) bool {
	if reqCtxBranch == "" || !eventBranch.IsSet() {
		return true
	}

	return strings.HasPrefix(reqCtxBranch, eventBranch.Or(""))
}

// isOtherAgentReply checks if the event came from another agent.
func isOtherAgentReply(currentAgentName string, ev *core.Event) bool {
	return currentAgentName != "" &&
		ev.Author != currentAgentName &&
		ev.Author != userAuthor
}

// presentOtherAgentMessage rewrites another agent's message as "user context"
// so the current agent can use it without confusion about authorship.
func presentOtherAgentMessage(ev *core.Event) *core.Message {
	if len(ev.Parts) == 0 {
		return nil
	}

	msg := &core.Message{
		Role:  core.RoleUser,
		Parts: []core.Part{&core.TextPart{Text: "For context:"}},
	}

	for _, part := range ev.Parts {
		switch p := part.(type) {
		case *core.TextPart:
			msg.Parts = append(msg.Parts,
				&core.TextPart{Text: "[" + ev.Author + "] said: " + p.Text})
		case *core.FunctionCallPart:
			msg.Parts = append(msg.Parts,
				&core.TextPart{
					Text: "[" + ev.Author + "] called tool `" + p.FunctionCall.Name +
						"` with parameters: " + fmt.Sprint(p.FunctionCall.Arguments),
				})
		case *core.FunctionResponsePart:
			msg.Parts = append(msg.Parts,
				&core.TextPart{
					Text: "[" + ev.Author + "] `" + p.FunctionResponse.Name +
						"` tool returned result: " + fmt.Sprint(p.FunctionResponse.Response),
				})
		default:
			// preserve unknown parts
			msg.Parts = append(msg.Parts, part)
		}
	}

	if len(msg.Parts) == 1 { // only "For context:", nothing meaningful
		return nil
	}

	return msg
}

// ProcessRequest adds messages to the request based on the agent's history settings.
func (p *MessagesProcessor) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	events := reqCtx.GetSessionHistory()
	historyLimit := agent.MaxHistoryMessages()
	mode := agent.HistoryMode()

	count := 0
	selected := make([]*core.Message, 0, historyLimit)

	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]

		if len(ev.Parts) == 0 || ev.IsPartial() || !belongsToBranch(reqCtx.Branch(), ev.Branch) {
			continue
		}

		// Filter based on history mode
		switch mode {
		case core.HistoryNone:
			// skip all past events, just take last user + context
			if isOtherAgentReply(agent.Name(), ev) {
				if msg := presentOtherAgentMessage(ev); msg != nil {
					selected = append(selected, msg)
					count++
				}
			} else if ev.Author == userAuthor {
				selected = append(selected, &core.Message{Role: ev.Role(), Parts: ev.Parts})
				count++
			}

			// Done: we don't walk further back
			i = -1

			continue
		case core.HistoryOwn:
			// include only user + this agent
			if ev.Author == userAuthor || ev.Author == agent.Name() {
				selected = append(selected, &core.Message{Role: ev.Role(), Parts: ev.Parts})
				count++
			}

		case core.HistoryAll:
			// include user + any agent (with context formatting for other agents)
			if isOtherAgentReply(agent.Name(), ev) {
				if msg := presentOtherAgentMessage(ev); msg != nil {
					selected = append(selected, msg)
					count++
				}
			} else {
				selected = append(selected, &core.Message{Role: ev.Role(), Parts: ev.Parts})
				count++
			}
		}

		if historyLimit > 0 && count >= historyLimit {
			break
		}
	}

	// Reverse for chronological order
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	req.Messages = selected

	return nil
}
