package viz

import (
	"context"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// GraphAdapter wraps a typed graph.Graph to implement the viz.Runnable interface.
// This adapter handles type conversion from HTTP input to graph input.
type GraphAdapter[I, O any] struct {
	g *graph.Graph[I, O]
}

// NewGraphAdapter creates an adapter for any compiled graph.
func NewGraphAdapter[I, O any](g *graph.Graph[I, O]) *GraphAdapter[I, O] {
	return &GraphAdapter[I, O]{g: g}
}

// Execute implements viz.Runnable by converting HTTP input and delegating to the graph.
func (a *GraphAdapter[I, O]) Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error] {
	// Convert HTTP input to typed input
	var typedInput I
	if convertedInput, ok := any(input).(I); ok {
		typedInput = convertedInput
	}
	// If conversion fails, use zero value (works for empty structs and reference types)

	// Execute and convert outputs to any
	return func(yield func(any, error) bool) {
		for output, err := range a.g.Run(ctx, typedInput, opts...) {
			if !yield(any(output), err) {
				return
			}
		}
	}
}

// GetNodes returns all node names in the graph.
func (a *GraphAdapter[I, O]) GetNodes() []string {
	return a.g.GetNodes()
}

// GetTopology returns the graph topology.
func (a *GraphAdapter[I, O]) GetTopology() *graph.Topology {
	return a.g.GetTopology()
}

// MermaidFlowchart generates a Mermaid diagram.
func (a *GraphAdapter[I, O]) MermaidFlowchart(direction string) string {
	return a.g.MermaidFlowchart(direction)
}

// MessageAdapter wraps message-based graphs (agents) to implement viz.Runnable.
// This adapter converts HTTP input to message slices and runs the agent.
type MessageAdapter struct {
	g *graph.Graph[[]message.Message, message.Message]
}

// NewMessageAdapter creates an adapter for message-based agents.
func NewMessageAdapter(g *graph.Graph[[]message.Message, message.Message]) *MessageAdapter {
	return &MessageAdapter{g: g}
}

// Execute implements viz.Runnable by converting HTTP input to messages.
func (a *MessageAdapter) Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error] {
	// Convert HTTP input to messages
	messages := a.convertInput(input)

	// Execute agent WITH server options to enable checkpointing and visualization
	return func(yield func(any, error) bool) {
		for msg, err := range a.g.Run(ctx, messages, opts...) {
			if !yield(any(msg), err) {
				return
			}
		}
	}
}

// convertInput converts HTTP input to message slice.
func (a *MessageAdapter) convertInput(input map[string]any) []message.Message {
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

// GetNodes returns node names from the agent's internal graph.
func (a *MessageAdapter) GetNodes() []string {
	return a.g.GetNodes()
}

// GetTopology returns topology from the agent's internal graph.
func (a *MessageAdapter) GetTopology() *graph.Topology {
	return a.g.GetTopology()
}

// MermaidFlowchart generates a diagram from the agent's internal graph.
func (a *MessageAdapter) MermaidFlowchart(direction string) string {
	return a.g.MermaidFlowchart(direction)
}
