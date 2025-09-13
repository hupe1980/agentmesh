package processor

import (
	"context"
	"strings"

	"github.com/hupe1980/agentmesh/core"
)

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

// ProcessRequest adds user content to the chat request.
func (p *MessagesProcessor) ProcessRequest(
	_ context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	// Add conversation history if available
	events := reqCtx.GetSessionHistory()
	historyLimit := agent.MaxHistoryMessages()

	// Collect matching events from newest to oldest, stopping once we have historyLimit (if > 0)
	var selected []*core.Message
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]

		if len(ev.Parts) == 0 {
			continue
		}

		if ev.IsPartial() {
			continue
		}

		if !belongsToBranch(reqCtx.Branch(), ev.Branch) {
			continue
		}

		selected = append(selected, &core.Message{Role: ev.Role(), Parts: ev.Parts})
		if historyLimit > 0 && len(selected) >= historyLimit {
			break
		}
	}

	// Reverse to restore chronological order (oldest first), then convert to messages
	messages := make([]*core.Message, 0, len(selected))
	for i := len(selected) - 1; i >= 0; i-- {
		m := selected[i]
		messages = append(messages, m)
	}

	req.Messages = messages

	return nil
}
