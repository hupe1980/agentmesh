package graph

import (
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNodes(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", "c")
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	nodes := compiled.GetNodes()
	assert.Len(t, nodes, 3)
	assert.Contains(t, nodes, "a")
	assert.Contains(t, nodes, "b")
	assert.Contains(t, nodes, "c")
}

func TestGetNodeInfo(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("node1", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("node2", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "node1")
	builder.AddEdge("node1", "node2")
	builder.AddEdge("node2", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	t.Run("valid node", func(t *testing.T) {
		info, err := compiled.GetNodeInfo("node1")
		require.NoError(t, err)
		assert.Equal(t, "node1", info.Name)
		assert.Equal(t, "standard", info.Type)
		// Edges from START are not counted as incoming edges (by design)
		assert.Equal(t, 0, info.IncomingEdges)
		assert.Equal(t, 1, info.OutgoingEdges)
		assert.False(t, info.IsConditional)
		assert.False(t, info.HasRetryPolicy)
	})

	t.Run("invalid node", func(t *testing.T) {
		_, err := compiled.GetNodeInfo("nonexistent")
		assert.ErrorIs(t, err, ErrNodeNotFound)
	})
}

func TestGetNodeInfo_WithRetryPolicy(t *testing.T) {
	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)

	err = g.AddNode(&Node{
		Name: "retryable",
		RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		},
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 3,
		},
	})
	require.NoError(t, err)

	g.AddEdge(StartNode, "retryable")
	g.AddEdge("retryable", EndNode)

	compiled, err := g.Compile()
	require.NoError(t, err)

	info, err := compiled.GetNodeInfo("retryable")
	require.NoError(t, err)
	assert.True(t, info.HasRetryPolicy)
	assert.Equal(t, 3, info.RetryMaxAttempts)
}

func TestGetAllNodeInfo(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	infos := compiled.GetAllNodeInfo()
	assert.Len(t, infos, 2)

	nodeNames := make(map[string]bool)
	for _, info := range infos {
		nodeNames[info.Name] = true
		assert.Equal(t, "standard", info.Type)
	}
	assert.True(t, nodeNames["a"])
	assert.True(t, nodeNames["b"])
}

func TestGetEdges(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddConditionalEdges("b", func(ctx context.Context, s stateif.Reader) []string {
		return []string{"c"}
	}, []string{"c", EndNode})
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	edges := compiled.GetEdges()
	assert.NotEmpty(t, edges)

	// Count edge types
	directEdges := 0
	conditionalEdges := 0
	for _, edge := range edges {
		if edge.Type == "direct" {
			directEdges++
		} else if edge.Type == "conditional" {
			conditionalEdges++
		}
	}
	assert.Greater(t, directEdges, 0)
	assert.Greater(t, conditionalEdges, 0)
}

func TestGetTopology(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("entry", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("middle", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("exit", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "entry")
	builder.AddEdge("entry", "middle")
	builder.AddEdge("middle", "exit")
	builder.AddEdge("exit", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	topo := compiled.GetTopology()
	assert.NotNil(t, topo)
	assert.Len(t, topo.Nodes, 3)
	assert.NotEmpty(t, topo.Edges)
	assert.Contains(t, topo.EntryPoints, "entry")
	assert.Contains(t, topo.ExitPoints, "exit")
	assert.Greater(t, topo.MaxDepth, 0)
}

func TestGetTopology_WithConditionals(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("router", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("path_a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("path_b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "router")
	builder.AddConditionalEdges("router", func(ctx context.Context, s stateif.Reader) []string {
		return []string{"path_a"}
	}, []string{"path_a", "path_b"})
	builder.AddEdge("path_a", EndNode)
	builder.AddEdge("path_b", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	topo := compiled.GetTopology()
	assert.Contains(t, topo.ConditionalNodes, "router")
	assert.Greater(t, topo.TotalPaths, 1)
}

func TestGetMetrics(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("a", "c") // Fan-out from 'a'
	builder.AddEdge("b", EndNode)
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	metrics := compiled.GetMetrics()
	assert.Equal(t, 3, metrics.TotalNodes)
	assert.Greater(t, metrics.TotalEdges, 0)
	assert.Greater(t, metrics.MaxFanOut, 1) // 'a' has fan-out of 2
	assert.NotNil(t, metrics.NodesByType)
	assert.Greater(t, metrics.NodesByType["standard"], 0)
}

func TestGetDependencies(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", "c")
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	t.Run("middle node", func(t *testing.T) {
		deps, err := compiled.GetDependencies("b")
		require.NoError(t, err)
		assert.Contains(t, deps.DirectPredecessors, "a")
		assert.Contains(t, deps.DirectSuccessors, "c")
		assert.Contains(t, deps.AllPredecessors, "a")
		assert.Contains(t, deps.AllSuccessors, "c")
		assert.Greater(t, deps.Depth, 0)
	})

	t.Run("first node", func(t *testing.T) {
		deps, err := compiled.GetDependencies("a")
		require.NoError(t, err)
		assert.Empty(t, deps.DirectPredecessors)
		assert.Contains(t, deps.DirectSuccessors, "b")
		assert.Len(t, deps.AllSuccessors, 2) // b and c
	})

	t.Run("invalid node", func(t *testing.T) {
		_, err := compiled.GetDependencies("nonexistent")
		assert.ErrorIs(t, err, ErrNodeNotFound)
	})
}

func TestGetExecutionPath(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	paths := compiled.GetExecutionPath(10)
	assert.NotEmpty(t, paths)

	// Verify path contains START, a, b, END
	foundValidPath := false
	for _, path := range paths {
		if len(path) == 4 && path[0] == StartNode && path[1] == "a" && path[2] == "b" && path[3] == EndNode {
			foundValidPath = true
			break
		}
	}
	assert.True(t, foundValidPath, "Should find path START -> a -> b -> END")
}

func TestGetExecutionPath_WithBranching(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("router", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("path_a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("path_b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "router")
	builder.AddConditionalEdges("router", func(ctx context.Context, s stateif.Reader) []string {
		return []string{"path_a"}
	}, []string{"path_a", "path_b"})
	builder.AddEdge("path_a", EndNode)
	builder.AddEdge("path_b", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	paths := compiled.GetExecutionPath(10)
	assert.NotEmpty(t, paths)
	assert.GreaterOrEqual(t, len(paths), 2, "Should have at least 2 paths (one for each branch)")
}

func TestCalculateDepth(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", "c")
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	assert.Equal(t, 1, compiled.calculateDepth("a"))
	assert.Equal(t, 2, compiled.calculateDepth("b"))
	assert.Equal(t, 3, compiled.calculateDepth("c"))
}

func TestFindAllPredecessors(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("d", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", "c")
	builder.AddEdge("a", "d")
	builder.AddEdge("d", "c")
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	predecessors := compiled.findAllPredecessors("c")
	assert.Contains(t, predecessors, "a")
	assert.Contains(t, predecessors, "b")
	assert.Contains(t, predecessors, "d")
}

func TestFindAllSuccessors(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{}, nil
	})

	builder.AddEdge(StartNode, "a")
	builder.AddEdge("a", "b")
	builder.AddEdge("b", "c")
	builder.AddEdge("c", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	successors := compiled.findAllSuccessors("a")
	assert.Contains(t, successors, "b")
	assert.Contains(t, successors, "c")
	assert.NotContains(t, successors, "a") // Should not contain itself
}

func TestCyclomaticComplexity(t *testing.T) {
	t.Run("linear graph", func(t *testing.T) {
		builder, err := NewBuilder()
		require.NoError(t, err)
		builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		})
		builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		})

		builder.AddEdge(StartNode, "a")
		builder.AddEdge("a", "b")
		builder.AddEdge("b", EndNode)

		compiled, err := builder.Compile()
		require.NoError(t, err)

		metrics := compiled.GetMetrics()
		// Linear graph: E=3, N=2, P=1 -> Complexity = 3-2+2=3
		assert.Greater(t, metrics.CyclomaticComplexity, 0)
	})

	t.Run("branching graph", func(t *testing.T) {
		builder, err := NewBuilder()
		require.NoError(t, err)
		builder.Node("a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		})
		builder.Node("b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		})
		builder.Node("c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		})

		builder.AddEdge(StartNode, "a")
		builder.AddEdge("a", "b")
		builder.AddEdge("a", "c")
		builder.AddEdge("b", EndNode)
		builder.AddEdge("c", EndNode)

		compiled, err := builder.Compile()
		require.NoError(t, err)

		metrics := compiled.GetMetrics()
		// Branching increases complexity
		assert.Greater(t, metrics.CyclomaticComplexity, 3)
	})
}
