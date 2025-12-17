package viz

import (
	"context"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// GraphAdapter wraps a graph.Graph to implement the viz.Runnable interface.
// This adapter handles type conversion from HTTP input to graph input.
type GraphAdapter struct {
	g *graph.Graph
}

// NewGraphAdapter creates an adapter for any compiled graph.
func NewGraphAdapter(g *graph.Graph) *GraphAdapter {
	return &GraphAdapter{g: g}
}

// Execute implements viz.Runnable by converting HTTP input and delegating to the graph.
func (a *GraphAdapter) Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error] {
	// Convert HTTP input to messages
	messages := convertInputToMessages(input)

	// Execute and convert outputs to any
	return func(yield func(any, error) bool) {
		for output, err := range a.g.Run(ctx, messages, opts...) {
			if !yield(any(output), err) {
				return
			}
		}
	}
}

// GetNodes returns all node names in the graph.
func (a *GraphAdapter) GetNodes() []string {
	return a.g.GetNodes()
}

// GetTopology returns the graph topology.
func (a *GraphAdapter) GetTopology() *graph.Topology {
	return a.g.GetTopology()
}

// MermaidFlowchart generates a Mermaid diagram.
func (a *GraphAdapter) MermaidFlowchart(direction string) string {
	return a.g.MermaidFlowchart(direction)
}

// convertInputToMessages converts HTTP input to message slice.
func convertInputToMessages(input map[string]any) []message.Message {
	if len(input) == 0 {
		// Empty input - provide default message
		return []message.Message{
			message.NewHumanMessageFromText("Hello! How can I help you today?"),
		}
	}

	// Extract content from input
	if content, ok := input["content"].(string); ok {
		return []message.Message{message.NewHumanMessageFromText(content)}
	}

	// Array of messages
	messages, ok := input["messages"].([]any)
	if !ok {
		// Fallback: stringify the entire input
		return []message.Message{
			message.NewHumanMessageFromText(fmt.Sprintf("%v", input)),
		}
	}

	result := make([]message.Message, 0, len(messages))
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := msgMap["content"].(string)
		if ok {
			result = append(result, message.NewHumanMessageFromText(content))
		}
	}
	if len(result) > 0 {
		return result
	}

	// Fallback: stringify the entire input
	return []message.Message{
		message.NewHumanMessageFromText(fmt.Sprintf("%v", input)),
	}
}
