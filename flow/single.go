package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow/processor"
)

// SingleAgentFlow implements a basic execution flow for a standalone agent
// (no transfers, no sub-agent delegation). It wires default processors for
// instruction resolution and message assembly, then relays model streaming
// events directly.
type SingleAgentFlow struct{ *BaseFlow }

// NewSingleAgentFlow creates a new basic single-agent flow.
func NewSingleAgentFlow(agent Agent, exec core.AgentExecutor) *SingleAgentFlow {
	baseFlow := NewBaseFlow(agent, exec)

	// Add default processors with adapters
	ip := processor.NewInstructionsProcessor()
	baseFlow.AddRequestProcessor(&requestProcessorAdapter{name: ip.Name(), fn: func(ctx context.Context, rc core.RequestContext, req *core.ModelRequest, ag Agent) error {
		return ip.ProcessRequest(ctx, rc, req, ag)
	}})
	mp := processor.NewMessagesProcessor()
	baseFlow.AddRequestProcessor(&requestProcessorAdapter{name: mp.Name(), fn: func(ctx context.Context, rc core.RequestContext, req *core.ModelRequest, ag Agent) error {
		return mp.ProcessRequest(ctx, rc, req, ag)
	}})

	return &SingleAgentFlow{BaseFlow: baseFlow}
}
