package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// Flow defines the interface for agent execution flows.
// A Flow processes the initial request, streams model output, optionally
// handles function calls, and may trigger agent transfers.
type Flow interface {
	Execute(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error
}

// Selector determines which flow to use based on agent capabilities.
// (Renamed from previous core.FlowSelector to keep public surface smaller.)
type Selector interface {
	SelectFlow(agent core.FlowAgent) Flow
}

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

// selector is the default Selector implementation.
type selector struct{ exec core.AgentExecutor }

// NewDefaultSelector creates a new default flow selector.
func NewDefaultSelector(exec core.AgentExecutor) Selector { return &selector{exec: exec} }

// SelectFlow chooses the appropriate flow for the given agent.
func (s *selector) SelectFlow(agent core.FlowAgent) Flow {
	// Use simple flow for isolated agents
	if !agent.IsTransferToParentEnabled() && !agent.IsTransferToPeersEnabled() && !agent.HasSubAgents() {
		return NewSingleAgentFlow(agent, s.exec)
	}

	// Use multi-agent flow for agents with transfer capabilities or sub-agents
	return NewMultiAgentFlow(agent, s.exec)
}
