package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
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
// Example:
//
//	g.AddConditionalEdges("model", RouteOnToolCalls("tool", graph.EndNode), []string{"tool", graph.EndNode})
func RouteOnToolCalls(ifToolCalls, otherwise string) func(context.Context, graph.StateReader) []string {
	return func(_ context.Context, s graph.StateReader) []string {
		transcript := s.MessagesSnapshot()
		if len(transcript) == 0 {
			return []string{otherwise}
		}

		last := transcript[len(transcript)-1]
		if last == nil {
			return []string{otherwise}
		}

		if ai, ok := last.(*message.AIMessage); ok {
			if len(ai.ToolCalls) > 0 {
				return []string{ifToolCalls}
			}
		}

		return []string{otherwise}
	}
}
