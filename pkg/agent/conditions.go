package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// RouteOnToolCalls creates a conditional routing function that checks if the last message
// in the conversation contains tool calls.
//
// Parameters:
//   - ifToolCalls: the node name to route to if tool calls are present
//   - otherwise: the node name to route to if no tool calls are present
//
// This is commonly used in agent graphs to determine whether to execute tools
// or finish the conversation.
//
// Note: This function is primarily used internally by agent implementations.
// For custom routing, use Command pattern with DeclaredTargets instead.
func RouteOnToolCalls(ifToolCalls, otherwise string) func(context.Context, state.ReadView) []string {
	return func(_ context.Context, view state.ReadView) []string {
		lastMsg := LastMessage(view)
		if lastMsg == nil {
			return []string{otherwise}
		}

		if ai, ok := lastMsg.(*message.AIMessage); ok {
			if len(ai.ToolCalls) > 0 {
				return []string{ifToolCalls}
			}
		}

		return []string{otherwise}
	}
}
