package flow

import "github.com/hupe1980/agentmesh/flow/processor"

// MultiAgentFlow orchestrates an agent that may perform tool calls and
// transfer control to sub-agents, enabling hierarchical or branching flows.
// It extends BaseFlow with processors suitable for multi-agent execution,
// including dynamic injection of the transfer_to_agent tool.
type MultiAgentFlow struct{ *BaseFlow }

// NewMultiAgentFlow creates a new multi-agent flow with default processors.
func NewMultiAgentFlow(agent Agent) *MultiAgentFlow {
	baseFlow := NewBaseFlow(agent)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(processor.NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(processor.NewMessagesProcessor())

	// Inject transfer_to_agent tool definition dynamically when applicable
	baseFlow.AddRequestProcessor(processor.NewTransferToolInjector())

	return &MultiAgentFlow{BaseFlow: baseFlow}
}
