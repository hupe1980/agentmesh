package flow

import "github.com/hupe1980/agentmesh/flow/processor"

// SingleAgentFlow implements a basic execution flow for a standalone agent
// (no transfers, no sub-agent delegation). It wires default processors for
// instruction resolution and message assembly, then relays model streaming
// events directly.
type SingleAgentFlow struct{ *BaseFlow }

// NewSingleAgentFlow creates a new basic single-agent flow.
func NewSingleAgentFlow(agent Agent) *SingleAgentFlow {
	baseFlow := NewBaseFlow(agent)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(processor.NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(processor.NewMessagesProcessor())

	return &SingleAgentFlow{BaseFlow: baseFlow}
}
