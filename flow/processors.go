package flow

import (
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	internalutil "github.com/hupe1980/agentmesh/internal/util"
	"github.com/hupe1980/agentmesh/model"
)

// InstructionsProcessor handles system prompt and instruction processing.
type InstructionsProcessor struct{}

// NewInstructionsProcessor creates a new instructions processor.
func NewInstructionsProcessor() *InstructionsProcessor { return &InstructionsProcessor{} }

// Name returns the processor's identifier.
func (p *InstructionsProcessor) Name() string { return "instructions" }

// ProcessRequest adds system instructions to the chat request.
func (p *InstructionsProcessor) ProcessRequest(invocationCtx *core.InvocationContext, req *model.Request, agent FlowAgent) error {
	instructions, err := agent.ResolveInstructions(invocationCtx)
	if err != nil {
		return fmt.Errorf("failed to resolve instruction: %w", err)
	}

	if invocationCtx.Logger != nil {
		if la, ok := agent.(interface{ GetName() string }); ok {
			invocationCtx.Logger.Debug("agent.instruction.resolved", "agent", la.GetName(), "length", len(instructions))
		}
	}

	if invocationCtx.Session != nil && invocationCtx.Session.State != nil {
		var tplErr error
		// Apply template substitution to system prompt using session state
		req.Instructions, tplErr = internalutil.RenderTemplate(instructions, invocationCtx.Session.State)
		if tplErr != nil {
			return fmt.Errorf("failed to render template: %w", tplErr)
		}
	} else {
		req.Instructions = instructions
	}

	return nil
}

type ContentsProcessor struct{}

// NewContentsProcessor creates a new contents processor.
func NewContentsProcessor() *ContentsProcessor { return &ContentsProcessor{} }

// Name returns the processor's identifier.
func (p *ContentsProcessor) Name() string { return "contents" }

// ProcessRequest adds user content to the chat request.
func (p *ContentsProcessor) ProcessRequest(invocationCtx *core.InvocationContext, req *model.Request, agent FlowAgent) error {
	contents := []core.Content{{
		Role:  "system",
		Parts: []core.Part{core.TextPart{Text: req.Instructions}},
	}}

	// Add conversation history if available
	if invocationCtx.Session != nil {
		events := invocationCtx.Session.GetConversationHistory()
		if len(events) > agent.MaxHistoryMessages() {
			events = events[len(events)-agent.MaxHistoryMessages():]
		}

		for _, ev := range events {
			if ev.Content != nil && len(ev.Content.Parts) > 0 {
				contents = append(contents, *ev.Content)
			}
		}
	}

	req.Contents = contents
	return nil
}
