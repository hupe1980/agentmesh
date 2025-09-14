package flow

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
	agent Agent,
) error {
	events := reqCtx.GetSessionHistory()
	historyLimit := agent.MaxHistoryMessages()

	// Walk backwards, collecting up to historyLimit
	count := 0
	selected := make([]*core.Message, 0, historyLimit)
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if len(ev.Parts) == 0 || ev.IsPartial() || !belongsToBranch(reqCtx.Branch(), ev.Branch) {
			continue
		}
		selected = append(selected, &core.Message{Role: ev.Role(), Parts: ev.Parts})
		count++
		if historyLimit > 0 && count >= historyLimit {
			break
		}
	}

	// Reverse in-place
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	req.Messages = selected

	return nil
}
