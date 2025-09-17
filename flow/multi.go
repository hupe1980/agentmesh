package flow

import "github.com/hupe1980/agentmesh/core"

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

	// Inject transfer_to_agent tool definition dynamically when applicable
	baseFlow.AddRequestProcessor(NewTransferToolInjector())

	return &MultiAgentFlow{BaseFlow: baseFlow}
}
