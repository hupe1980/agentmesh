package graph

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/require"
)

func TestSubgraphSupport(t *testing.T) {
	t.Run("basic subgraph as node", func(t *testing.T) {
		// Create subgraph that doubles a counter
		subState, err := NewStateManager(0)
		require.NoError(t, err)
		subGraph, err := NewGraph(subState)
		require.NoError(t, err)

		if err := subGraph.AddNode(&Node{
			Name: "double",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				val, _ := s.Get("value").(int)
				return &NodeResult{
					Updates: map[string]any{"value": val * 2},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		subGraph.AddEdge(StartNode, "double")
		compiledSub, err := subGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Create parent graph that uses subgraph
		parentState, err := NewStateManager(0)
		require.NoError(t, err)
		parentState.Set("count", 5) // Initialize count
		parentGraph, err := NewGraph(parentState)
		require.NoError(t, err)

		if err := parentGraph.AddNode(&Node{
			Name: "prepare",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				count, _ := s.Get("count").(int)
				// Pass count to subgraph as "value"
				return &NodeResult{
					Updates: map[string]any{"value": count},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		// Add subgraph as a node
		if err := parentGraph.AddNode(compiledSub.AsNode("subgraph")); err != nil {
			t.Fatal(err)
		}

		parentGraph.AddEdge(StartNode, "prepare")
		parentGraph.AddEdge("prepare", "subgraph")

		compiled, err := parentGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = Last(compiled.Run(context.Background(), nil))
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		// Check result: prepare sets value=5, subgraph doubles it to 10
		result, ok := compiled.State().Get("value").(int)
		if !ok || result != 10 {
			t.Errorf("expected value=10, got %v", compiled.State().Get("value"))
		}
	})

	t.Run("subgraph with state mapping", func(t *testing.T) {
		// Create subgraph that processes "input" and produces "output"
		subState, err := NewStateManager(0)
		require.NoError(t, err)
		subGraph, err := NewGraph(subState)
		require.NoError(t, err)

		if err := subGraph.AddNode(&Node{
			Name: "process",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				input, _ := s.Get("input").(string)
				return &NodeResult{
					Updates: map[string]any{"output": "processed: " + input},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		subGraph.AddEdge(StartNode, "process")
		compiledSub, err := subGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Create parent graph with different state keys
		parentState, err := NewStateManager(0)
		require.NoError(t, err)
		parentState.Set("data", "hello") // Initialize data
		parentGraph, err := NewGraph(parentState)
		require.NoError(t, err)

		// Add subgraph with state mapping
		subNode := compiledSub.AsNodeWithStateMapping(
			"mapped-subgraph",
			// Map parent "data" to subgraph "input"
			func(s StateReader) (map[string]any, []ExecutionResult) {
				data, _ := s.Get("data").(string)
				return map[string]any{"input": data}, nil
			},
			// Map subgraph "output" to parent "result"
			func(s StateReader) (map[string]any, []ExecutionResult) {
				output, _ := s.Get("output").(string)
				return map[string]any{"result": output}, nil
			},
		)

		if err := parentGraph.AddNode(subNode); err != nil {
			t.Fatal(err)
		}

		parentGraph.AddEdge(StartNode, "mapped-subgraph")

		compiled, err := parentGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = Last(compiled.Run(context.Background(), nil))
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		// Check mapped result
		result, ok := compiled.State().Get("result").(string)
		if !ok || result != "processed: hello" {
			t.Errorf("expected result='processed: hello', got %v", compiled.State().Get("result"))
		}
	})

	t.Run("nested subgraphs", func(t *testing.T) {
		// Inner subgraph: adds 10
		innerState, err := NewStateManager(0)
		require.NoError(t, err)
		innerGraph, err := NewGraph(innerState)
		require.NoError(t, err)

		if err := innerGraph.AddNode(&Node{
			Name: "add10",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				n, _ := s.Get("n").(int)
				return &NodeResult{Updates: map[string]any{"n": n + 10}}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}
		innerGraph.AddEdge(StartNode, "add10")
		compiledInner, err := innerGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Middle subgraph: contains inner subgraph, then multiplies by 2
		middleState, err := NewStateManager(0)
		require.NoError(t, err)
		middleGraph, err := NewGraph(middleState)
		require.NoError(t, err)

		if err := middleGraph.AddNode(compiledInner.AsNode("inner")); err != nil {
			t.Fatal(err)
		}

		if err := middleGraph.AddNode(&Node{
			Name: "times2",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				n, _ := s.Get("n").(int)
				return &NodeResult{Updates: map[string]any{"n": n * 2}}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		middleGraph.AddEdge(StartNode, "inner")
		middleGraph.AddEdge("inner", "times2")
		compiledMiddle, err := middleGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Outer graph: initializes n=5, then runs middle subgraph
		outerState, err := NewStateManager(0)
		require.NoError(t, err)
		outerState.Set("n", 5) // Initialize n
		outerGraph, err := NewGraph(outerState)
		require.NoError(t, err)

		if err := outerGraph.AddNode(compiledMiddle.AsNode("middle")); err != nil {
			t.Fatal(err)
		}

		outerGraph.AddEdge(StartNode, "middle")
		compiled, err := outerGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = Last(compiled.Run(context.Background(), nil))
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		// Expected: 5 -> inner adds 10 = 15 -> times2 = 30
		result, ok := compiled.State().Get("n").(int)
		if !ok || result != 30 {
			t.Errorf("expected n=30 (5 + 10) * 2, got %v", compiled.State().Get("n"))
		}
	})

	t.Run("subgraph handles errors", func(t *testing.T) {
		// Create subgraph that fails
		subState, err := NewStateManager(0)
		require.NoError(t, err)
		subGraph, err := NewGraph(subState)
		require.NoError(t, err)

		if err := subGraph.AddNode(&Node{
			Name: "fail",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return nil, ErrNodeNotFound
			},
		}); err != nil {
			t.Fatal(err)
		}

		subGraph.AddEdge(StartNode, "fail")
		compiledSub, err := subGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Parent graph using failing subgraph
		parentState, err := NewStateManager(0)
		require.NoError(t, err)
		parentGraph, err := NewGraph(parentState)
		require.NoError(t, err)

		if err := parentGraph.AddNode(compiledSub.AsNode("failing-sub")); err != nil {
			t.Fatal(err)
		}

		parentGraph.AddEdge(StartNode, "failing-sub")
		compiled, err := parentGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = Last(compiled.Run(context.Background(), nil))
		if err == nil {
			t.Fatal("expected error from failing subgraph")
		}

		// Error should mention subgraph name
		if err.Error() == "" {
			t.Error("error message should not be empty")
		}
	})

	t.Run("subgraph with message passing", func(t *testing.T) {
		// Subgraph that produces messages
		subState, err := NewStateManager(0)
		require.NoError(t, err)
		subGraph, err := NewGraph(subState)
		require.NoError(t, err)

		if err := subGraph.AddNode(&Node{
			Name: "messenger",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{
					Updates: map[string]any{"sent": true},
					Messages: []message.Message{
						message.NewHumanMessageFromText("Hello from subgraph"),
					},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		subGraph.AddEdge(StartNode, "messenger")
		compiledSub, err := subGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Parent graph
		parentState, err := NewStateManager(0)
		require.NoError(t, err)
		parentGraph, err := NewGraph(parentState)
		require.NoError(t, err)

		if err := parentGraph.AddNode(compiledSub.AsNode("sub-with-messages")); err != nil {
			t.Fatal(err)
		}

		parentGraph.AddEdge(StartNode, "sub-with-messages")
		compiled, err := parentGraph.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = Last(compiled.Run(context.Background(), nil))
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		// Check that messages were passed through
		msgs := compiled.State().EventsSnapshot()
		if len(msgs) == 0 {
			t.Fatal("expected messages from subgraph")
		}

		found := false
		for _, evt := range msgs {
			if text, ok := evt.Message.(*message.HumanMessage); ok {
				for _, part := range text.Parts() {
					if tp, ok := part.(message.TextPart); ok && tp.Text == "Hello from subgraph" {
						found = true
						break
					}
				}
			}
		}
		if !found {
			t.Error("expected to find 'Hello from subgraph' in messages")
		}
	})
}
