package viz

import (
	"context"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// GraphAdapter wraps a typed graph.Compiled to implement the viz.Runnable interface.
// This adapter handles type conversion from HTTP input to graph input.
type GraphAdapter[I, O any] struct {
	compiled *graph.Compiled[I, O]
}

// NewGraphAdapter creates an adapter for any compiled graph.
func NewGraphAdapter[I, O any](compiled *graph.Compiled[I, O]) *GraphAdapter[I, O] {
	return &GraphAdapter[I, O]{compiled: compiled}
}

// Execute implements viz.Runnable by converting HTTP input and delegating to the compiled graph.
func (a *GraphAdapter[I, O]) Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error] {
	// Convert HTTP input to typed input
	var typedInput I
	if convertedInput, ok := any(input).(I); ok {
		typedInput = convertedInput
	}
	// If conversion fails, use zero value (works for empty structs and reference types)

	// Execute and convert outputs to any
	return func(yield func(any, error) bool) {
		for output, err := range a.compiled.Run(ctx, typedInput, opts...) {
			if !yield(any(output), err) {
				return
			}
		}
	}
}

// GetNodes returns all node names in the graph.
func (a *GraphAdapter[I, O]) GetNodes() []string {
	return a.compiled.GetNodes()
}

// GetTopology returns the graph topology.
func (a *GraphAdapter[I, O]) GetTopology() *graph.Topology {
	return a.compiled.GetTopology()
}

// MermaidFlowchart generates a Mermaid diagram.
func (a *GraphAdapter[I, O]) MermaidFlowchart(direction string) string {
	return a.compiled.MermaidFlowchart(direction)
}

// MessageAdapter wraps message-based runnables (agents) to implement viz.Runnable.
// This adapter converts HTTP input to message slices and runs the agent WITHOUT
// passing server execution options, as agents manage their own execution.
type MessageAdapter struct {
	runnable graph.Runnable[[]message.Message, message.Message]
}

// NewMessageAdapter creates an adapter for message-based agents.
func NewMessageAdapter(runnable graph.Runnable[[]message.Message, message.Message]) *MessageAdapter {
	return &MessageAdapter{runnable: runnable}
}

// Execute implements viz.Runnable by converting HTTP input to messages.
func (a *MessageAdapter) Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error] {
	// Convert HTTP input to messages
	messages := a.convertInput(input)

	// Execute agent WITH server options to enable checkpointing and visualization
	// The opts include RunID, Checkpointer, and other execution options needed for debugging
	return func(yield func(any, error) bool) {
		for msg, err := range a.runnable.Run(ctx, messages, opts...) {
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
	// Try to extract nodes using type assertion
	type nodeProvider interface {
		GetNodes() []string
	}
	if provider, ok := a.runnable.(nodeProvider); ok {
		return provider.GetNodes()
	}
	return []string{"agent"}
}

// GetTopology returns topology from the agent's internal graph.
func (a *MessageAdapter) GetTopology() *graph.Topology {
	// Try to extract topology using type assertion
	type topologyProvider interface {
		GetTopology() *graph.Topology
	}
	if provider, ok := a.runnable.(topologyProvider); ok {
		return provider.GetTopology()
	}
	return nil
}

// MermaidFlowchart generates a diagram from the agent's internal graph.
func (a *MessageAdapter) MermaidFlowchart(direction string) string {
	// Try to extract mermaid using type assertion
	type mermaidProvider interface {
		MermaidFlowchart(string) string
	}
	if provider, ok := a.runnable.(mermaidProvider); ok {
		return provider.MermaidFlowchart(direction)
	}
	return "graph LR\n    Start[Agent] --> End[Output]"
}
