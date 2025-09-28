package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// BasicProcessor handles system prompt and instruction processing.
type BasicProcessor struct{}

// NewBasicProcessor creates a new instructions processor.
func NewBasicProcessor() *BasicProcessor { return &BasicProcessor{} }

// Name returns the processor's identifier.
func (p *BasicProcessor) Name() string { return "instructions" }

// ProcessRequest adds system instructions to the chat request.
func (p *BasicProcessor) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	req.OutputSchema = agent.OutputSchema()

	return nil
}
