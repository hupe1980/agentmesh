package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow/processor"
)

// MultiAgentFlow orchestrates an agent that may perform tool calls and
// transfer control to sub-agents, enabling hierarchical or branching flows.
// It extends BaseFlow with processors suitable for multi-agent execution,
// including dynamic injection of the transfer_to_agent tool.
type MultiAgentFlow struct{ *BaseFlow }

// NewMultiAgentFlow creates a new multi-agent flow with default processors.
func NewMultiAgentFlow(agent Agent, exec core.AgentExecutor) *MultiAgentFlow {
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
	tp := processor.NewTransferToolInjector()
	baseFlow.AddRequestProcessor(&requestProcessorAdapter{name: tp.Name(), fn: func(ctx context.Context, rc core.RequestContext, req *core.ModelRequest, ag Agent) error {
		return tp.ProcessRequest(ctx, rc, req, ag)
	}})

	return &MultiAgentFlow{BaseFlow: baseFlow}
}
