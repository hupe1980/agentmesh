package graph

import (
	"context"
	"testing"

	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConditionalGating verifies that the conditional edge gating logic
// correctly distinguishes between:
// 1. Nodes with static edges + conditional edges (should NOT be gated)
// 2. Nodes with ONLY conditional edges (should be gated)
func TestConditionalGating(t *testing.T) {
	t.Run("node with static edge is not gated despite conditional", func(t *testing.T) {
		// This tests the fix for the conditional self-loop bug.
		// A node reachable via static edge should execute even if it also
		// has conditional edges pointing to it.

		state, err := NewStateManager(0)
		require.NoError(t, err)
		state.Set("executed", false)

		g, err := NewGraph(state)
		require.NoError(t, err)

		require.NoError(t, g.AddNode(&Node{
			Name: "target",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				return &NodeResult{
					Updates: map[string]any{"executed": true},
				}, nil
			},
		}))

		// Static edge: START -> target
		g.AddEdge(StartNode, "target")

		// Conditional edge ALSO points to target (but condition prevents loop)
		g.AddConditionalEdges("target", func(ctx context.Context, s stateif.Reader) []string {
			executed, _ := s.Get("executed").(bool)
			if executed {
				return []string{EndNode} // Stop after first execution
			}
			return []string{"target"} // Would loop (but never happens due to execution order)
		}, []string{"target", EndNode})

		compiled, err := g.Compile()
		require.NoError(t, err)

		// Verify topology: target should NOT be gated
		topo := compiled.topology()
		assert.False(t, topo.ConditionalGate["target"],
			"target should NOT be gated - it has a static edge from START")

		// Execute: target should run because it's not gated
		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)

		executed, ok := compiled.State().Get("executed").(bool)
		require.True(t, ok)
		assert.True(t, executed, "target should have executed via static edge")
	})

	t.Run("node with only conditional edges is gated", func(t *testing.T) {
		// A node that is ONLY reachable via conditional edges should be gated
		// and only execute when the condition activates it.

		state, err := NewStateManager(0)
		require.NoError(t, err)
		state.Set("should_activate", false)

		g, err := NewGraph(state)
		require.NoError(t, err)

		entryExecuted := false
		require.NoError(t, g.AddNode(&Node{
			Name: "entry",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				entryExecuted = true
				return &NodeResult{}, nil
			},
		}))

		conditionalExecuted := false
		require.NoError(t, g.AddNode(&Node{
			Name: "conditional_only",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				conditionalExecuted = true
				return &NodeResult{}, nil
			},
		}))

		// Static edge: START -> entry
		g.AddEdge(StartNode, "entry")

		// ONLY conditional edge to conditional_only (no static edges)
		g.AddConditionalEdges("entry", func(ctx context.Context, s stateif.Reader) []string {
			shouldActivate, _ := s.Get("should_activate").(bool)
			if shouldActivate {
				return []string{"conditional_only"}
			}
			return []string{EndNode}
		}, []string{"conditional_only", EndNode})

		compiled, err := g.Compile()
		require.NoError(t, err)

		// Verify topology: conditional_only SHOULD be gated
		topo := compiled.topology()
		assert.True(t, topo.ConditionalGate["conditional_only"],
			"conditional_only should be gated - it has NO static edges")

		// Execute with should_activate=false
		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)
		assert.True(t, entryExecuted, "entry should have executed")
		assert.False(t, conditionalExecuted, "conditional_only should NOT execute when condition is false")
	})

	t.Run("node with static edge from non-START is not gated", func(t *testing.T) {
		// A node with a static edge from another node (not START) should
		// also NOT be gated, even if it has conditional edges.

		state, err := NewStateManager(0)
		require.NoError(t, err)

		g, err := NewGraph(state)
		require.NoError(t, err)

		require.NoError(t, g.AddNode(&Node{
			Name: "first",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		}))

		secondExecuted := false
		require.NoError(t, g.AddNode(&Node{
			Name: "second",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				secondExecuted = true
				return &NodeResult{}, nil
			},
		}))

		// Static edges: START -> first -> second
		g.AddEdge(StartNode, "first")
		g.AddEdge("first", "second")

		// Conditional edge also points to second
		g.AddConditionalEdges("first", func(ctx context.Context, s stateif.Reader) []string {
			return []string{} // Don't activate conditionally
		}, []string{"second"})

		compiled, err := g.Compile()
		require.NoError(t, err)

		// Verify topology: second should NOT be gated
		topo := compiled.topology()
		assert.False(t, topo.ConditionalGate["second"],
			"second should NOT be gated - it has a static edge from first")

		// Execute: second should run via static edge
		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)
		assert.True(t, secondExecuted, "second should have executed via static edge")
	})

	t.Run("multiple nodes with mixed static and conditional edges", func(t *testing.T) {
		// Complex scenario: multiple nodes with various edge configurations

		state, err := NewStateManager(0)
		require.NoError(t, err)

		g, err := NewGraph(state)
		require.NoError(t, err)

		executions := make(map[string]bool)

		addNode := func(name string) {
			require.NoError(t, g.AddNode(&Node{
				Name: name,
				RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
					executions[name] = true
					return &NodeResult{}, nil
				},
			}))
		}

		addNode("A") // Has static from START, has conditional loop
		addNode("B") // Has static from A, has conditional
		addNode("C") // Only conditional from A
		addNode("D") // Static from B

		// Static edges
		g.AddEdge(StartNode, "A")
		g.AddEdge("A", "B")
		g.AddEdge("B", "D")

		// Conditional edges
		g.AddConditionalEdges("A", func(ctx context.Context, s stateif.Reader) []string {
			return []string{} // Don't activate
		}, []string{"A", "C"}) // Self-loop + C

		g.AddConditionalEdges("B", func(ctx context.Context, s stateif.Reader) []string {
			return []string{} // Don't activate
		}, []string{"C"})

		compiled, err := g.Compile()
		require.NoError(t, err)

		// Verify topology
		topo := compiled.topology()
		assert.False(t, topo.ConditionalGate["A"], "A has static edge from START")
		assert.False(t, topo.ConditionalGate["B"], "B has static edge from A")
		assert.True(t, topo.ConditionalGate["C"], "C has ONLY conditional edges")
		assert.False(t, topo.ConditionalGate["D"], "D has static edge from B")

		// Execute
		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)

		// A, B, D should execute via static edges; C should NOT (gated)
		assert.True(t, executions["A"], "A should execute")
		assert.True(t, executions["B"], "B should execute")
		assert.False(t, executions["C"], "C should NOT execute (gated)")
		assert.True(t, executions["D"], "D should execute")
	})

	t.Run("conditional self-loop works correctly", func(t *testing.T) {
		// This is the specific bug that was fixed: a node with a static
		// edge AND a conditional self-loop should execute.

		state, err := NewStateManager(0)
		require.NoError(t, err)
		state.Set("counter", 0)

		g, err := NewGraph(state)
		require.NoError(t, err)

		require.NoError(t, g.AddNode(&Node{
			Name: "looper",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				counter, _ := s.Get("counter").(int)
				return &NodeResult{
					Updates: map[string]any{"counter": counter + 1},
				}, nil
			},
		}))

		// Static edge: START -> looper
		g.AddEdge(StartNode, "looper")

		// Conditional self-loop: looper -> looper (limited by condition)
		g.AddConditionalEdges("looper", func(ctx context.Context, s stateif.Reader) []string {
			counter, _ := s.Get("counter").(int)
			if counter < 3 {
				return []string{"looper"} // Loop back
			}
			return []string{EndNode} // Stop
		}, []string{"looper", EndNode})

		compiled, err := g.Compile()
		require.NoError(t, err)

		// Verify topology: looper should NOT be gated
		topo := compiled.topology()
		assert.False(t, topo.ConditionalGate["looper"],
			"looper should NOT be gated - it has static edge from START")

		// Execute: should loop 3 times then stop
		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)

		counter, ok := compiled.State().Get("counter").(int)
		require.True(t, ok)
		assert.Equal(t, 3, counter, "looper should have executed 3 times")
	})
}
