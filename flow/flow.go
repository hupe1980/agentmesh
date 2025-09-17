package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// RequestProcessor processes the request before sending it to the LLM.
type RequestProcessor interface {
	// Name returns the processor's identifier.
	Name() string

	// ProcessRequest modifies the chat request before LLM execution.
	ProcessRequest(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent core.FlowAgent) error
}

// ResponseProcessor processes the response after receiving it from the LLM.
type ResponseProcessor interface {
	// Name returns the processor's identifier.
	Name() string

	// ProcessResponse handles the LLM response and may generate additional events.
	ProcessResponse(ctx context.Context, reqCtx core.RequestContext, resp *core.ModelResponse, agent core.FlowAgent) error
}

// selector is the default core.FlowSelector implementation.
type selector struct{ executors *Executors }

// NewDefaultSelector creates a new default flow selector.
func NewDefaultSelector(executors *Executors) core.FlowSelector {
	return &selector{executors: executors}
}

// Select chooses the appropriate flow for the given agent.
func (s *selector) Select(agent core.FlowAgent) core.Flow {
	// Use simple flow for isolated agents
	if !agent.IsTransferToParentEnabled() && !agent.IsTransferToPeersEnabled() && !agent.HasSubAgents() {
		return NewSingleAgentFlow(agent, s.executors)
	}

	// Use multi-agent flow for agents with transfer capabilities or sub-agents
	return NewMultiAgentFlow(agent, s.executors)
}
