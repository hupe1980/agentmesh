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

// SingleAgentFlow implements a basic execution flow for a standalone agent
// (no transfers, no sub-agent delegation). It wires default processors for
// instruction resolution and message assembly, then relays model streaming
// events directly.
type SingleAgentFlow struct{ *BaseFlow }

// NewSingleAgentFlow creates a new basic single-agent flow.
func NewSingleAgentFlow(agent core.FlowAgent, executors *Executors) *SingleAgentFlow {
	baseFlow := NewBaseFlow(agent, executors)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(NewMessagesProcessor())
	baseFlow.AddRequestProcessor(NewOutputSchemaProcessor())

	return &SingleAgentFlow{BaseFlow: baseFlow}
}

// MultiAgentFlow orchestrates an agent that may perform tool calls and
// transfer control to sub-agents, enabling hierarchical or branching flows.
// It extends BaseFlow with processors suitable for multi-agent execution,
// including dynamic injection of the transfer_to_agent tool.
type MultiAgentFlow struct{ *BaseFlow }

// NewMultiAgentFlow creates a new multi-agent flow with default processors.
func NewMultiAgentFlow(agent core.FlowAgent, executors *Executors) *MultiAgentFlow {
	baseFlow := NewBaseFlow(agent, executors)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(NewMessagesProcessor())
	baseFlow.AddRequestProcessor(NewOutputSchemaProcessor())

	// Inject transfer_to_agent tool definition dynamically when applicable
	baseFlow.AddRequestProcessor(NewTransferToolInjector())

	return &MultiAgentFlow{BaseFlow: baseFlow}
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
