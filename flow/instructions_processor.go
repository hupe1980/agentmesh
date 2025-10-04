package flow

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/prompt"
	"github.com/hupe1980/agentmesh/logging"
)

// InstructionsProcessor handles system prompt and instruction processing.
type InstructionsProcessor struct{}

// NewInstructionsProcessor creates a new instructions processor.
func NewInstructionsProcessor() *InstructionsProcessor { return &InstructionsProcessor{} }

// Name returns the processor's identifier.
func (p *InstructionsProcessor) Name() string { return "instructions" }

// ProcessRequest adds system instructions to the chat request.
func (p *InstructionsProcessor) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	log := logging.FromContext(ctx)

	instructions, err := agent.ResolveInstructions(ctx, reqCtx)
	if err != nil {
		return fmt.Errorf("failed to resolve instruction: %w", err)
	}

	log.Debug("agent.instruction.resolved", "agent", agent.Name(), "length", len(instructions))

	// Apply template substitution using a merged state snapshot (persisted + delta)
	snapshot := reqCtx.StateSnapshot()
	if len(snapshot) > 0 {
		rendered, tplErr := prompt.Render(instructions, snapshot)
		if tplErr != nil {
			return fmt.Errorf("failed to render template: %w", tplErr)
		}
		req.AppendInstructions(rendered)
	} else {
		req.AppendInstructions(instructions)
	}

	return nil
}
