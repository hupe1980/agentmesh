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
	g.AddNode(&BaseCommandNode{
		NodeName:        "start_node",
		DeclaredTargets: NewTargetSet("process"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("process"), nil
		},
	})
	g.AddNode(&BaseCommandNode{
		NodeName:        "process",
		DeclaredTargets: NewTargetSet("end_node"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("end_node"), nil
		},
	})
	g.AddNode(&BaseCommandNode{
		NodeName:        "end_node",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
	})

	// Set entry point (edges defined via DeclaredTargets)
	if err := g.SetEntryPoint("start_node"); err != nil {
		return nil, err
	}

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
	g.AddNode(&BaseCommandNode{
		NodeName:        "retryable",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
		Retry: retryPolicy,
	})

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

	// Should have 1 direct edge (from SetEntryPoint) + 3 command edges (all nodes have DeclaredTargets)
	assert.Len(t, edges, 4)

	// Count edge types
	directEdges := 0
	commandEdges := 0
	for _, edge := range edges {
		if edge.Type == "direct" {
			directEdges++
		}
		if edge.Type == "command" {
			commandEdges++
		}
	}
	assert.Equal(t, 1, directEdges, "should have 1 direct edge from SetEntryPoint")
	assert.Equal(t, 3, commandEdges, "should have 3 command edges (start_node, process, end_node)")
}

func TestGraph_GetEdges_WithConditionals(t *testing.T) {
	mgr := newTestManager()
	g, _ := NewGraph(mgr)

	g.AddNode(&BaseCommandNode{
		NodeName:        "router",
		DeclaredTargets: NewTargetSet("option_a", "option_b"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("option_a"), nil
		},
	})
	g.AddNode(&BaseCommandNode{
		NodeName:        "option_a",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
	})
	g.AddNode(&BaseCommandNode{
		NodeName:        "option_b",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
	})

	if err := g.SetEntryPoint("router"); err != nil {
		t.Fatal(err)
	}

	edges := g.GetEdges()

	// Should have 1 entry edge + 3 command edges (router, option_a, option_b)
	assert.Len(t, edges, 4)

	commandEdges := 0
	for _, edge := range edges {
		if edge.Type == "command" {
			commandEdges++
		}
	}
	assert.Equal(t, 3, commandEdges, "should have Command edges for router, option_a, and option_b")
}

func TestGraph_GetTopology(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	topo := g.GetTopology()
	require.NotNil(t, topo)

	assert.Len(t, topo.Nodes, 3)
	// Should have 1 direct edge (from SetEntryPoint) + 3 command edges (all nodes have DeclaredTargets)
	assert.Len(t, topo.Edges, 4)
	assert.Equal(t, []string{"start_node"}, topo.EntryPoints)
	assert.Equal(t, []string{"end_node"}, topo.ExitPoints)
	// All nodes have Command routing, so they're in CommandNodes
	assert.Len(t, topo.CommandNodes, 3, "all nodes use Command pattern")
	assert.Empty(t, topo.IsolatedNodes)
	assert.Greater(t, topo.MaxDepth, 0)
	assert.Equal(t, 1, topo.TotalPaths)
}

func TestGraph_GetTopology_WithConditionals(t *testing.T) {
	mgr := newTestManager()
	g, _ := NewGraph(mgr)

	g.AddNode(&BaseCommandNode{
		NodeName:        "router",
		DeclaredTargets: NewTargetSet("high_priority", "normal"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("high_priority"), nil
		},
	})
	g.AddNode(&BaseCommandNode{
		NodeName:        "high_priority",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
	})
	g.AddNode(&BaseCommandNode{
		NodeName:        "normal",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
	})

	if err := g.SetEntryPoint("router"); err != nil {
		t.Fatal(err)
	}

	topo := g.GetTopology()

	// In Command pattern, all nodes with DeclaredTargets are command nodes
	assert.Contains(t, topo.CommandNodes, "router")
	assert.Greater(t, topo.TotalPaths, 1)
}

func TestGraph_GetMetrics(t *testing.T) {
	g, err := createTestGraph()
	require.NoError(t, err)

	metrics := g.GetMetrics()
	require.NotNil(t, metrics)

	assert.Equal(t, 3, metrics.TotalNodes)
	assert.Equal(t, 4, metrics.TotalEdges)
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

func TestMermaidFlowchart_NoDuplicateEdges(t *testing.T) {
	// Test that MermaidFlowchart doesn't generate duplicate edges
	// This was a bug where edges were added twice in the loop
	mgr := newTestManager()
	g, err := NewGraph(mgr)
	require.NoError(t, err)

	// Create a graph with multiple nodes and branches
	g.AddNode(&BaseCommandNode{
		NodeName:        "router",
		DeclaredTargets: NewTargetSet("handler_a", "handler_b"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("handler_a"), nil
		},
	})

	g.AddNode(&BaseCommandNode{
		NodeName:        "handler_a",
		DeclaredTargets: NewTargetSet("aggregator"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("aggregator"), nil
		},
	})

	g.AddNode(&BaseCommandNode{
		NodeName:        "handler_b",
		DeclaredTargets: NewTargetSet("aggregator"),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return GotoOne("aggregator"), nil
		},
	})

	g.AddNode(&BaseCommandNode{
		NodeName:        "aggregator",
		DeclaredTargets: NewTargetSet(EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*Command, error) {
			return End(nil), nil
		},
	})

	g.SetEntryPoint("router")

	// Compile the graph
	executor := NewMessagePregelExecutor()
	compiled, err := Compile(g, executor)
	require.NoError(t, err)

	// Generate Mermaid flowchart
	flowchart := compiled.MermaidFlowchart("TD")

	// Split into lines and count unique edges
	lines := make(map[string]int)
	for _, line := range splitLines(flowchart) {
		trimmed := trimSpace(line)
		if containsEdgeArrow(trimmed) {
			lines[trimmed]++
		}
	}

	// Check that each edge appears exactly once
	for line, count := range lines {
		assert.Equal(t, 1, count, "Edge should appear exactly once: %s", line)
	}

	// Verify expected edges are present
	assert.Contains(t, flowchart, "__start__ --> router")
	assert.Contains(t, flowchart, "router -.-> handler_a")
	assert.Contains(t, flowchart, "router -.-> handler_b")
	assert.Contains(t, flowchart, "handler_a -.-> aggregator")
	assert.Contains(t, flowchart, "handler_b -.-> aggregator")
	assert.Contains(t, flowchart, "aggregator -.-> __end__")

	// Count occurrences of each edge to ensure no duplicates
	assert.Equal(t, 1, countOccurrences(flowchart, "__start__ --> router"))
	assert.Equal(t, 1, countOccurrences(flowchart, "router -.-> handler_a"))
	assert.Equal(t, 1, countOccurrences(flowchart, "router -.-> handler_b"))
	assert.Equal(t, 1, countOccurrences(flowchart, "handler_a -.-> aggregator"))
	assert.Equal(t, 1, countOccurrences(flowchart, "handler_b -.-> aggregator"))
	assert.Equal(t, 1, countOccurrences(flowchart, "aggregator -.-> __end__"))
}

// Helper functions for the test
func splitLines(s string) []string {
	lines := []string{}
	current := ""
	for _, ch := range s {
		if ch == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func trimSpace(s string) string {
	// Simple trim implementation
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func containsEdgeArrow(s string) bool {
	return contains(s, "-->") || contains(s, "-.->")
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func countOccurrences(s, substr string) int {
	count := 0
	pos := 0
	for {
		idx := findSubstring(s[pos:], substr)
		if idx == -1 {
			break
		}
		count++
		pos += idx + len(substr)
	}
	return count
}

func findSubstring(s, substr string) int {
	if len(substr) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
