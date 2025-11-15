package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompiled_MessageRunnable tests the most common use case:
// a message-based agent that processes []message.Message and returns state.ExecutionResult.
func TestCompiled_MessageRunnable(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	// Add a simple echo node
	builder.AddNode(&graph.Node{
		Name: "echo",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			// Get input messages
			msgs := state.ExtractMessages(s.MessagesSnapshot())

			// Echo back the last message
			if len(msgs) > 0 {
				lastMsg := msgs[len(msgs)-1]
				response := message.NewAIMessageFromText("Echo: " + extractTextFromMsg(lastMsg))
				return &graph.NodeResult{
					Messages: []message.Message{response},
				}, nil
			}

			return &graph.NodeResult{}, nil
		},
	})

	builder.AddEdge(graph.StartNode, "echo")
	builder.AddEdge("echo", graph.EndNode)

	// Compile with generic types (MessageRunnable)
	compiled, err := graph.Compile[[]message.Message, state.ExecutionResult](builder)
	require.NoError(t, err)
	require.NotNil(t, compiled)

	// Verify it implements MessageRunnable
	var _ graph.MessageRunnable = compiled

	// Execute with type-safe input (no type assertions!)
	input := []message.Message{
		message.NewHumanMessageFromText("Hello, world!"),
	}

	ctx := context.Background()
	results, err := graph.Collect(compiled.Run(ctx, input))
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Verify output is state.ExecutionResult (type-safe!)
	lastResult := results[len(results)-1]
	assert.Contains(t, extractTextFromMsg(lastResult.Message), "Echo:")
}

// TestCompiled_StateRunnable tests a state-based graph that processes
// map[string]any and returns state.ExecutionResult.
func TestCompiled_StateRunnable(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	builder.AddNode(&graph.Node{
		Name: "process",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			// Get input state
			value := s.Get("counter")
			counter := 0
			if v, ok := value.(int); ok {
				counter = v
			}

			// Increment counter
			counter++

			msg := message.NewAIMessageFromText("Counter incremented")
			return &graph.NodeResult{
				Updates:  map[string]any{"counter": counter},
				Messages: []message.Message{msg},
			}, nil
		},
	})

	builder.AddEdge(graph.StartNode, "process")
	builder.AddEdge("process", graph.EndNode)

	// Compile with state-based types
	compiled, err := graph.Compile[map[string]any, state.ExecutionResult](builder)
	require.NoError(t, err)

	// Verify it implements StateRunnable
	var _ graph.StateRunnable = compiled

	// Execute with state input
	input := map[string]any{"counter": 5}

	ctx := context.Background()
	results, err := graph.Collect(compiled.Run(ctx, input))
	require.NoError(t, err)
	require.NotEmpty(t, results)
}

// TestCompiled_StringRunnable tests a simple text-to-text transformation.
func TestCompiled_StringRunnable(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	builder.AddNode(&graph.Node{
		Name: "uppercase",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			msgs := state.ExtractMessages(s.MessagesSnapshot())
			if len(msgs) > 0 {
				text := extractTextFromMsg(msgs[len(msgs)-1])
				response := message.NewAIMessageFromText(text + " processed")
				return &graph.NodeResult{
					Messages: []message.Message{response},
				}, nil
			}
			return &graph.NodeResult{}, nil
		},
	})

	builder.AddEdge(graph.StartNode, "uppercase")
	builder.AddEdge("uppercase", graph.EndNode)

	// Compile with string types
	compiled, err := graph.Compile[string, string](builder)
	require.NoError(t, err)

	// Verify it implements StringRunnable
	var _ graph.StringRunnable = compiled

	// Execute with string input
	ctx := context.Background()
	results, err := graph.CollectGeneric(compiled.Run(ctx, "hello"))
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Output is string (type-safe!)
	lastResult := results[len(results)-1]
	assert.Contains(t, lastResult, "processed")
}

// TestCompiled_TypeSafety ensures compile-time type safety.
func TestCompiled_TypeSafety(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	builder.AddNode(&graph.Node{
		Name: "noop",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})
	builder.AddEdge(graph.StartNode, "noop")
	builder.AddEdge("noop", graph.EndNode)

	// Compile with specific types
	compiled, err := graph.Compile[[]message.Message, state.ExecutionResult](builder)
	require.NoError(t, err)

	// This should compile - correct input type
	input := []message.Message{message.NewHumanMessageFromText("test")}
	_, err = graph.Collect(compiled.Run(context.Background(), input))
	require.NoError(t, err)

	// Note: These would be COMPILE ERRORS (caught by IDE/compiler):
	// compiled.Run(context.Background(), "wrong type")  // Error: cannot use string as []message.Message
	// compiled.Run(context.Background(), 123)           // Error: cannot use int as []message.Message
}

// TestCompiled_StateAccess tests that State() method works correctly.
func TestCompiled_StateAccess(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	builder.AddNode(&graph.Node{
		Name: "setter",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"key": "value"},
			}, nil
		},
	})
	builder.AddEdge(graph.StartNode, "setter")
	builder.AddEdge("setter", graph.EndNode)

	compiled, err := graph.Compile[[]message.Message, state.ExecutionResult](builder)
	require.NoError(t, err)

	// Execute
	input := []message.Message{message.NewHumanMessageFromText("test")}
	_, err = graph.Collect(compiled.Run(context.Background(), input))
	require.NoError(t, err)

	// Access state
	stateManager := compiled.State()
	require.NotNil(t, stateManager)

	value := stateManager.Get("key")
	assert.Equal(t, "value", value)
}

// TestCompiled_CurrentSuperstep tests superstep tracking.
func TestCompiled_CurrentSuperstep(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	builder.AddNode(&graph.Node{
		Name: "node",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})
	builder.AddEdge(graph.StartNode, "node")
	builder.AddEdge("node", graph.EndNode)

	compiled, err := graph.Compile[[]message.Message, state.ExecutionResult](builder)
	require.NoError(t, err)

	// Initially 0
	assert.Equal(t, int64(0), compiled.CurrentSuperstep())

	// Execute
	input := []message.Message{message.NewHumanMessageFromText("test")}
	_, err = graph.Collect(compiled.Run(context.Background(), input))
	require.NoError(t, err)

	// Superstep tracking exists (value >= 0)
	assert.GreaterOrEqual(t, compiled.CurrentSuperstep(), int64(0))
}

// TestCompiled_MustCompile tests the panic version.
func TestCompiled_MustCompile(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	builder.AddNode(&graph.Node{
		Name: "node",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})
	builder.AddEdge(graph.StartNode, "node")
	builder.AddEdge("node", graph.EndNode)

	// Should not panic
	compiled := graph.MustCompile[[]message.Message, state.ExecutionResult](builder)
	require.NotNil(t, compiled)
}

// TestCompiled_MustCompile_Panic tests that invalid graphs panic.
func TestCompiled_MustCompile_Panic(t *testing.T) {
	builder, err := graph.NewBuilder()
	require.NoError(t, err)

	// Create invalid graph (edge to non-existent node)
	builder.AddEdge(graph.StartNode, "nonexistent")

	// Should panic
	assert.Panics(t, func() {
		graph.MustCompile[[]message.Message, state.ExecutionResult](builder)
	})
}

// Helper function to extract text from a message
func extractTextFromMsg(msg message.Message) string {
	if msg == nil {
		return ""
	}
	var texts []string
	for _, part := range msg.Parts() {
		if textPart, ok := part.(message.TextPart); ok {
			texts = append(texts, textPart.Text)
		}
	}
	return texts[0]
}
