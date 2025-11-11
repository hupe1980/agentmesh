package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConditionalSeesNodeOutput verifies that conditional edges can route based on
// the output of the node that just executed (state is committed before evaluation).
func TestConditionalSeesNodeOutput(t *testing.T) {
	state := NewStateManager(0)
	g := NewGraph(state)

	// Node that sets a decision value
	g.AddNode(&Node{
		Name: "decide",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{
				Updates: map[string]any{
					"decision": "go_left",
				},
			}, nil
		},
	})

	// Two target nodes
	g.AddNode(&Node{
		Name: "left",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{
				Updates: map[string]any{"result": "left_executed"},
			}, nil
		},
	})

	g.AddNode(&Node{
		Name: "right",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{
				Updates: map[string]any{"result": "right_executed"},
			}, nil
		},
	})

	// Conditional edge that routes based on the decision value
	g.AddConditionalEdges("decide", func(ctx context.Context, s StateReader) []string {
		decision := s.Get("decision")
		if decision == "go_left" {
			return []string{"left"}
		}
		return []string{"right"}
	}, []string{"left", "right"})

	g.AddEdge(StartNode, "decide")
	g.AddEdge("left", EndNode)
	g.AddEdge("right", EndNode)

	compiled, err := g.Compile()
	require.NoError(t, err)

	ctx := context.Background()
	_, err = Last(compiled.Run(ctx, nil))
	require.NoError(t, err)

	// Verify that "left" was executed (because decision was "go_left")
	result := compiled.State().Get("result")
	assert.Equal(t, "left_executed", result, "conditional should have routed to 'left' based on node output")

	// Verify decision was set
	decision := compiled.State().Get("decision")
	assert.Equal(t, "go_left", decision)
}

// TestConditionalSeesUpdatedState verifies that conditionals see state updates
// from multiple nodes in sequence.
func TestConditionalSeesUpdatedState(t *testing.T) {
	state := NewStateManager(0)
	g := NewGraph(state)

	// First node sets counter to 1
	g.AddNode(&Node{
		Name: "increment1",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{
				Updates: map[string]any{"counter": 1},
			}, nil
		},
	})

	// Second node increments counter
	g.AddNode(&Node{
		Name: "increment2",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			current := s.Get("counter")
			count := 0
			if current != nil {
				if c, ok := current.(int); ok {
					count = c
				}
			}
			return &NodeResult{
				Updates: map[string]any{"counter": count + 1},
			}, nil
		},
	})

	// Two target nodes
	g.AddNode(&Node{
		Name: "path_a",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{Updates: map[string]any{"path": "a"}}, nil
		},
	})

	g.AddNode(&Node{
		Name: "path_b",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{Updates: map[string]any{"path": "b"}}, nil
		},
	})

	g.AddEdge(StartNode, "increment1")
	g.AddEdge("increment1", "increment2")

	// Conditional routes based on counter value (should be 2 at this point)
	g.AddConditionalEdges("increment2", func(ctx context.Context, s StateReader) []string {
		counter := s.Get("counter")
		if counter != nil {
			if c, ok := counter.(int); ok && c >= 2 {
				return []string{"path_b"}
			}
		}
		return []string{"path_a"}
	}, []string{"path_a", "path_b"})

	g.AddEdge("path_a", EndNode)
	g.AddEdge("path_b", EndNode)

	compiled, err := g.Compile()
	require.NoError(t, err)

	ctx := context.Background()
	_, err = Last(compiled.Run(ctx, nil))
	require.NoError(t, err)

	// Verify that path_b was taken (counter was 2)
	path := compiled.State().Get("path")
	assert.Equal(t, "b", path, "conditional should have seen counter=2 and routed to path_b")

	counter := compiled.State().Get("counter")
	assert.Equal(t, 2, counter)
}

// TestConditionalWithMultipleOutputs verifies conditionals work with multiple target selection.
func TestConditionalWithMultipleOutputs(t *testing.T) {
	state := NewStateManager(0)
	g := NewGraph(state)

	// Node that produces multiple flags
	g.AddNode(&Node{
		Name: "analyze",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{
				Updates: map[string]any{
					"needs_validation": true,
					"needs_logging":    true,
				},
			}, nil
		},
	})

	// Multiple target nodes
	g.AddNode(&Node{
		Name: "validate",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{Updates: map[string]any{"validated": true}}, nil
		},
	})

	g.AddNode(&Node{
		Name: "log",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{Updates: map[string]any{"logged": true}}, nil
		},
	})

	g.AddEdge(StartNode, "analyze")

	// Conditional that can activate multiple targets based on flags
	g.AddConditionalEdges("analyze", func(ctx context.Context, s StateReader) []string {
		var targets []string
		if s.Get("needs_validation") == true {
			targets = append(targets, "validate")
		}
		if s.Get("needs_logging") == true {
			targets = append(targets, "log")
		}
		return targets
	}, []string{"validate", "log"})

	g.AddEdge("validate", EndNode)
	g.AddEdge("log", EndNode)

	compiled, err := g.Compile()
	require.NoError(t, err)

	ctx := context.Background()
	_, err = Last(compiled.Run(ctx, nil))
	require.NoError(t, err)

	// Both paths should have executed in parallel
	validated := compiled.State().Get("validated")
	assert.Equal(t, true, validated, "validate node should have executed")

	logged := compiled.State().Get("logged")
	assert.Equal(t, true, logged, "log node should have executed")
}
