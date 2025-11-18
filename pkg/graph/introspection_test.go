package graph

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test-local dummy key for testing - graph tests don't need message-specific types
var testDummyKey = state.NewListKey[string]("__test_data__", 0)

// Helper function for tests
func newTestManager() *state.Manager {
	mgr := state.NewManager()
	state.RegisterListKey(mgr, testDummyKey)
	return mgr
}

func createTestGraph() (*Graph, error) {
	mgr := newTestManager()

	g, err := NewGraph(mgr)
	if err != nil {
		return nil, err
	}

	// Add nodes
	g.AddNode(NewBaseNode("start_node", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}))
	g.AddNode(NewBaseNode("process", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}))
	g.AddNode(NewBaseNode("end_node", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}))

	// Add edges
	g.AddEdge(StartNode, "start_node")
	g.AddEdge("start_node", "process")
	g.AddEdge("process", "end_node")
	g.AddEdge("end_node", EndNode)

	return g, nil
}

func TestGraph_GetNodes(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	nodes := g.GetNodes()

	assert.Len(t, nodes, 3)
	assert.Contains(t, nodes, "start_node")
	assert.Contains(t, nodes, "process")
	assert.Contains(t, nodes, "end_node")
}

func TestGraph_GetNodeInfo(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	t.Run("ValidNode", func(t *testing.T) {
		info, err := g.GetNodeInfo("process")
		require.NoError(t, err)
		require.NotNil(t, info)

		assert.Equal(t, "process", info.Name)
		assert.Equal(t, "standard", info.Type)
		assert.Equal(t, 1, info.IncomingEdges)
		assert.Equal(t, 1, info.OutgoingEdges)
		assert.False(t, info.IsConditional)
		assert.False(t, info.IsConditionalGate)
		assert.False(t, info.HasRetryPolicy)
	})

	t.Run("NodeNotFound", func(t *testing.T) {
		_, err := g.GetNodeInfo("nonexistent")
		assert.Error(t, err)
	})
}

func TestExportToMermaid_ComplexFlowWithBranches(t *testing.T) {
	mgr := newTestManager()

	g, _ := NewGraph(mgr)

	retryPolicy := NewRetryPolicy().WithMaxAttempts(5).Build()
	g.AddNode(NewBaseNodeWithRetry("retryable", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}, retryPolicy))

	info, err := g.GetNodeInfo("retryable")
	require.NoError(t, err)

	assert.True(t, info.HasRetryPolicy)
	assert.Equal(t, 5, info.RetryMaxAttempts)
}

func TestGraph_GetAllNodeInfo(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	allInfo := g.GetAllNodeInfo()

	assert.Len(t, allInfo, 3)

	// Should be sorted by name
	assert.Equal(t, "end_node", allInfo[0].Name)
	assert.Equal(t, "process", allInfo[1].Name)
	assert.Equal(t, "start_node", allInfo[2].Name)
}

func TestGraph_GetEdges(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	edges := g.GetEdges()

	assert.Len(t, edges, 4)

	// Check that all edges are direct (no conditionals)
	for _, edge := range edges {
		assert.Equal(t, "direct", edge.Type)
	}
}

func TestGraph_GetEdges_WithConditionals(t *testing.T) {
	mgr := newTestManager()
	g, _ := NewGraph(mgr)

	g.AddNode(NewBaseNode("router", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}))
	g.AddNode(NewBaseNode("option_a", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}))
	g.AddNode(NewBaseNode("option_b", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
		return nil, nil
	}))

	g.AddEdge(StartNode, "router")
	g.AddConditionalEdges("router", func(ctx context.Context, s *state.ReadView) []string {
		return []string{"option_a"}
	}, []string{"option_a", "option_b"})

	edges := g.GetEdges()

	// Should have 1 direct edge + 1 conditional edge
	assert.Len(t, edges, 2)

	conditionalEdges := 0
	for _, edge := range edges {
		if edge.Type == "conditional" {
			conditionalEdges++
			assert.Equal(t, "router", edge.From)
			assert.Contains(t, edge.ConditionalTargets, "option_a")
			assert.Contains(t, edge.ConditionalTargets, "option_b")
		}
	}
	assert.Equal(t, 1, conditionalEdges)
}

func TestGraph_GetTopology(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	topo := g.GetTopology()
	require.NotNil(t, topo)

	assert.Len(t, topo.Nodes, 3)
	assert.Len(t, topo.Edges, 4)
	assert.Equal(t, []string{"start_node"}, topo.EntryPoints)
	assert.Equal(t, []string{"end_node"}, topo.ExitPoints)
	assert.Empty(t, topo.ConditionalNodes)
	assert.Empty(t, topo.IsolatedNodes)
	assert.Greater(t, topo.MaxDepth, 0)
	assert.Equal(t, 1, topo.TotalPaths)
}

func TestGraph_GetTopology_WithConditionals(t *testing.T) {
	mgr := newTestManager()
	g, _ := NewGraph(mgr)

	g.AddNode(NewBaseNode("router", nil))
	g.AddNode(NewBaseNode("high_priority", nil))
	g.AddNode(NewBaseNode("normal", nil))

	g.AddEdge(StartNode, "router")
	g.AddConditionalEdges("router", func(ctx context.Context, s *state.ReadView) []string {
		return []string{"high_priority"}
	}, []string{"high_priority", "normal"})
	g.AddEdge("high_priority", EndNode)
	g.AddEdge("normal", EndNode)

	topo := g.GetTopology()

	assert.Contains(t, topo.ConditionalNodes, "router")
	assert.Greater(t, topo.TotalPaths, 1)
}

func TestGraph_GetMetrics(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	metrics := g.GetMetrics()
	require.NotNil(t, metrics)

	assert.Equal(t, 3, metrics.TotalNodes)
	assert.Equal(t, 4, metrics.TotalEdges)
	assert.Equal(t, 0, metrics.ConditionalEdges)
	assert.Greater(t, metrics.AverageFanOut, 0.0)
	assert.Greater(t, metrics.AverageFanIn, 0.0)
	assert.Contains(t, metrics.NodesByType, "standard")
	assert.Equal(t, 3, metrics.NodesByType["standard"])
}

func TestGraph_GetNodeDependencies(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	t.Run("MiddleNode", func(t *testing.T) {
		deps, err := g.GetNodeDependencies("process")
		require.NoError(t, err)
		require.NotNil(t, deps)

		assert.Equal(t, "process", deps.Node)
		assert.Equal(t, []string{"start_node"}, deps.DirectPredecessors)
		assert.Equal(t, []string{"end_node"}, deps.DirectSuccessors)
		assert.Contains(t, deps.AllPredecessors, "start_node")
		assert.Contains(t, deps.AllSuccessors, "end_node")
		assert.Greater(t, deps.Depth, 0)
	})

	t.Run("StartNode", func(t *testing.T) {
		deps, err := g.GetNodeDependencies("start_node")
		require.NoError(t, err)

		// start_node has START as a predecessor
		assert.Contains(t, deps.DirectPredecessors, StartNode)
		assert.NotEmpty(t, deps.DirectSuccessors)
	})

	t.Run("NodeNotFound", func(t *testing.T) {
		_, err := g.GetNodeDependencies("nonexistent")
		assert.Error(t, err)
	})
}

func TestGraph_CalculateDepth(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	// START -> start_node (depth 1) -> process (depth 2) -> end_node (depth 3)
	depthStartNode := g.calculateDepth("start_node")
	depthProcess := g.calculateDepth("process")
	depthEndNode := g.calculateDepth("end_node")

	assert.Greater(t, depthStartNode, 0)
	assert.Greater(t, depthProcess, depthStartNode)
	assert.Greater(t, depthEndNode, depthProcess)
}
