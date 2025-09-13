package flow

import (
	"github.com/hupe1980/agentmesh/core"
)

// SingleAgentFlow implements a basic execution flow for a standalone agent
// (no transfers, no sub-agent delegation). It wires default processors for
// instruction resolution and message assembly, then relays model streaming
// events directly.
type SingleAgentFlow struct{ *BaseFlow }

// NewSingleAgentFlow creates a new basic single-agent flow.
func NewSingleAgentFlow(agent Agent, exec core.AgentExecutor) *SingleAgentFlow {
	baseFlow := NewBaseFlow(agent, exec)

	// Add default processors for advanced functionality
	baseFlow.AddRequestProcessor(NewInstructionsProcessor())
	baseFlow.AddRequestProcessor(NewMessagesProcessor())

	return &SingleAgentFlow{BaseFlow: baseFlow}
}
