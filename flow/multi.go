package flow

// MultiAgentFlow orchestrates an agent that may perform tool calls and
// transfer control to sub-agents, enabling hierarchical / branching flows.
// MultiAgentFlow extends BaseFlow by selecting processors suitable for
// multi-agent graph execution.
type MultiAgentFlow struct{ *BaseFlow }

// NewMultiAgentFlow creates a new auto flow with default processors.
func NewMultiAgentFlow(agent FlowAgent) *MultiAgentFlow {
	baseFlow := NewBaseFlow(agent)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(NewContentsProcessor())
	// Inject transfer_to_agent tool definition dynamically when applicable
	baseFlow.AddRequestProcessor(NewTransferToolInjector())

	return &MultiAgentFlow{BaseFlow: baseFlow}
}
