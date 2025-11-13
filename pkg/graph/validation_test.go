package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphValidation(t *testing.T) {
	t.Run("detects unreachable node", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "reachable",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "unreachable",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "reachable")
		g.AddEdge("reachable", EndNode)
		// "unreachable" has no incoming edges

		_, err = g.Compile()
		require.Error(t, err, "should detect unreachable node")
		assert.ErrorIs(t, err, ErrUnreachableNode)
		assert.Contains(t, err.Error(), "unreachable")
	})

	t.Run("allows valid graph", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "step1",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "step2",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "step1")
		g.AddEdge("step1", "step2")
		g.AddEdge("step2", EndNode)

		_, err = g.Compile()
		require.NoError(t, err, "valid graph should compile without errors")
	})

	t.Run("unreachable node via conditional branch is reachable", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "router",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "path_a",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "path_b",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "router")
		g.AddConditionalEdges("router", func(ctx context.Context, s StateReader) []string {
			return []string{"path_a"}
		}, []string{"path_a", "path_b"})
		g.AddEdge("path_a", EndNode)
		g.AddEdge("path_b", EndNode)

		// Both path_a and path_b should be reachable (conditional makes them both possible)
		_, err = g.Compile()
		require.NoError(t, err, "graph with conditional branches should compile")
	})

	t.Run("detects missing edge target", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "existing",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "existing")
		g.AddEdge("existing", "missing-node") // Target doesn't exist

		_, err = g.Compile()
		require.Error(t, err, "should detect missing edge target")
		assert.Contains(t, err.Error(), "missing-node", "error should mention the missing node")
	})
}
