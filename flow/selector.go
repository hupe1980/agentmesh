package flow

// FlowSelector determines which flow to use based on agent capabilities.
//
// This implements the Google ADK pattern where the flow is selected
// dynamically based on the agent's configuration.
type FlowSelector struct{}

// NewFlowSelector creates a new flow selector.
func NewFlowSelector() *FlowSelector { return &FlowSelector{} }

// SelectFlow chooses the appropriate flow for the given agent.
//
// Selection logic mirrors Google ADK's approach:
//   - SingleAgentFlow for isolated agents without transfers or sub-agents
//   - MultiAgentFlow for agents with transfer capabilities or sub-agents
func (fs *FlowSelector) SelectFlow(agent FlowAgent) Flow {
	// Use simple flow for isolated agents
	if !agent.IsTransferEnabled() && len(agent.GetSubAgents()) == 0 {
		return NewSingleAgentFlow(agent)
	}
	// Use auto flow for agents with advanced capabilities
	return NewMultiAgentFlow(agent)
}
