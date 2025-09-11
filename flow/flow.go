package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// Flow defines the interface for agent execution flows.
// A Flow processes the initial request, streams model output, optionally
// handles function calls, and may trigger agent transfers.
type Flow interface {
	// Execute runs the flow with the given context, request context, and event queue.
	// Implementations must write events via queue.Write, guaranteeing persistence ordering.
	Execute(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error
}

// Agent is an alias to the core.FlowAgent contract.
type Agent = core.FlowAgent

// RequestProcessor processes the request before sending it to the LLM.
type RequestProcessor interface {
	// Name returns the processor's identifier.
	Name() string

	// ProcessRequest modifies the chat request before LLM execution.
	ProcessRequest(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent Agent) error
}

// ResponseProcessor processes the response after receiving it from the LLM.
type ResponseProcessor interface {
	// Name returns the processor's identifier.
	Name() string

	// ProcessResponse handles the LLM response and may generate additional events.
	ProcessResponse(ctx context.Context, reqCtx core.RequestContext, resp *core.ModelResponse, agent Agent) error
}

// Selector determines which flow to use based on agent capabilities.
// Implementations may choose different policies.
type Selector interface {
	// SelectFlow chooses the appropriate flow for the given agent.
	SelectFlow(agent Agent) Flow
}

// selector is the default Selector implementation.
type selector struct{}

// NewDefaultSelector creates a new default flow selector.
func NewDefaultSelector() Selector { return &selector{} }

// SelectFlow chooses the appropriate flow for the given agent.
func (s *selector) SelectFlow(agent Agent) Flow {
	// Use simple flow for isolated agents
	if !agent.IsTransferToParentEnabled() && !agent.IsTransferToPeersEnabled() && !agent.HasSubAgents() {
		return NewSingleAgentFlow(agent)
	}

	// Use multi-agent flow for agents with transfer capabilities or sub-agents
	return NewMultiAgentFlow(agent)
}
