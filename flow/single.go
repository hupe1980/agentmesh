package flow

// SingleAgentFlow implements a basic execution flow for a standalone agent
// (no transfers, no sub-agent delegation). It wires default processors for
// instruction resolution and content assembly, then relays model streaming
// events directly.
type SingleAgentFlow struct{ *BaseFlow }

// NewSingleAgentFlow creates a new basic single-agent flow.
func NewSingleAgentFlow(agent FlowAgent) *SingleAgentFlow {
	baseFlow := NewBaseFlow(agent)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(NewContentsProcessor())

	return &SingleAgentFlow{BaseFlow: baseFlow}
}
