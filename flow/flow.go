package flow

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// Agent represents the orchestration-facing view of an agent used by flows and processors.
type Agent interface {
	core.AgentIdentity
	core.HierarchicalAgent
	ResolveInstructions(ctx context.Context, roCtx core.ReadonlyContext) (string, error)
	Model() core.Model
	Tools() []core.Tool
	MaxHistoryMessages() int
	IsFunctionCallingEnabled() bool
	IsStreamingEnabled() bool
	IsTransferToPeersEnabled() bool
	IsTransferToParentEnabled() bool
	OutputKey() string
}

// Flow defines the interface for agent execution flows.
// A Flow processes the initial request, streams model output, optionally
// handles function calls, and may trigger agent transfers.
type Flow interface {
	Execute(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error
}

// Selector determines which flow to use based on agent capabilities.
type Selector interface{ SelectFlow(agent Agent) Flow }

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

// selector is the default Selector implementation.
type selector struct{ executors *Executors }

// NewDefaultSelector creates a new default flow selector.
func NewDefaultSelector(executors *Executors) Selector { return &selector{executors: executors} }

// SelectFlow chooses the appropriate flow for the given agent.
func (s *selector) SelectFlow(agent Agent) Flow {
	// Use simple flow for isolated agents
	if !agent.IsTransferToParentEnabled() && !agent.IsTransferToPeersEnabled() && !agent.HasSubAgents() {
		return NewSingleAgentFlow(agent, s.executors)
	}

	// Use multi-agent flow for agents with transfer capabilities or sub-agents
	return NewMultiAgentFlow(agent, s.executors)
}
