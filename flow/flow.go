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
type Selector interface{ SelectFlow(agent Agent) Flow }

// Agent is the orchestration-facing contract (formerly core.FlowAgent) used by flows and processors.
// It intentionally omits the executable Run method to avoid cyclic deps and restrict what flows can do.
type Agent interface {
	core.AgentIdentity
	core.HierarchicalAgent

	ResolveInstructions(ctx context.Context, roCtx core.ReadonlyContext) (string, error)
	Model() core.Model
	Tools() map[string]core.Tool
	MaxHistoryMessages() int
	IsFunctionCallingEnabled() bool
	IsStreamingEnabled() bool
	IsTransferToPeersEnabled() bool
	IsTransferToParentEnabled() bool
	OutputKey() string
}

// RequestProcessor processes the request before sending it to the LLM.
type RequestProcessor interface {
	// Name returns the processor's identifier.
	Name() string

	// ProcessRequest modifies the chat request before LLM execution.
	ProcessRequest(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent Agent) error
}

// requestProcessorAdapter allows using a processor that expects a narrower agent view.
type requestProcessorAdapter struct {
	name string
	fn   func(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent Agent) error
}

func (a *requestProcessorAdapter) Name() string { return a.name }
func (a *requestProcessorAdapter) ProcessRequest(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent Agent) error {
	return a.fn(ctx, reqCtx, req, agent)
}

// ResponseProcessor processes the response after receiving it from the LLM.
type ResponseProcessor interface {
	// Name returns the processor's identifier.
	Name() string

	// ProcessResponse handles the LLM response and may generate additional events.
	ProcessResponse(ctx context.Context, reqCtx core.RequestContext, resp *core.ModelResponse, agent Agent) error
}

// selector is the default Selector implementation.
type selector struct{ exec core.AgentExecutor }

// NewDefaultSelector creates a new default flow selector.
func NewDefaultSelector(exec core.AgentExecutor) Selector { return &selector{exec: exec} }

// SelectFlow chooses the appropriate flow for the given agent.
func (s *selector) SelectFlow(agent Agent) Flow {
	// Use simple flow for isolated agents
	if !agent.IsTransferToParentEnabled() && !agent.IsTransferToPeersEnabled() && !agent.HasSubAgents() {
		return NewSingleAgentFlow(agent, s.exec)
	}

	// Use multi-agent flow for agents with transfer capabilities or sub-agents
	return NewMultiAgentFlow(agent, s.exec)
}
