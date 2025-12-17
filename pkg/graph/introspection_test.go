package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNodes(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("a", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("b")
	}, "b")
	g.Node("b", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	compiled, err := g.Build()
	require.NoError(t, err)

	nodes := compiled.GetNodes()
	assert.Equal(t, []string{"a", "b"}, nodes)
}

func TestGetNodeInfo(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("entry", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("middle")
	}, "middle")
	g.Node("middle", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("exit")
	}, "exit")
	g.Node("exit", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("entry")

	compiled, err := g.Build()
	require.NoError(t, err)

	t.Run("entry node", func(t *testing.T) {
		info, err := compiled.GetNodeInfo("entry")
		require.NoError(t, err)
		assert.Equal(t, "entry", info.Name)
		assert.Equal(t, "entry", info.Type)
		assert.True(t, info.IsEntryPoint)
		assert.Equal(t, 1, info.IncomingEdges) // START -> entry
		assert.Equal(t, 1, info.OutgoingEdges)
		assert.Equal(t, []string{"middle"}, info.DeclaredTargets)
	})

	t.Run("middle node", func(t *testing.T) {
		info, err := compiled.GetNodeInfo("middle")
		require.NoError(t, err)
		assert.Equal(t, "middle", info.Name)
		assert.Equal(t, "standard", info.Type)
		assert.False(t, info.IsEntryPoint)
		assert.Equal(t, 1, info.IncomingEdges)
		assert.Equal(t, 1, info.OutgoingEdges)
	})

	t.Run("exit node", func(t *testing.T) {
		info, err := compiled.GetNodeInfo("exit")
		require.NoError(t, err)
		assert.Equal(t, "exit", info.Name)
		assert.Equal(t, "terminal", info.Type)
		assert.False(t, info.IsEntryPoint)
		assert.Equal(t, 1, info.IncomingEdges)
		assert.Equal(t, 1, info.OutgoingEdges)
		assert.Contains(t, info.DeclaredTargets, graph.END)
	})

	t.Run("non-existent node", func(t *testing.T) {
		_, err := compiled.GetNodeInfo("nonexistent")
		assert.Error(t, err)
	})
}

func TestGetNodeInfoWithInterrupt(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("step", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("step")
	g.InterruptBefore("step")

	compiled, err := g.Build()
	require.NoError(t, err)

	info, err := compiled.GetNodeInfo("step")
	require.NoError(t, err)
	assert.True(t, info.HasInterrupt)
}

func TestGetTopology(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("a", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("b")
	}, "b")
	g.Node("b", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	compiled, err := g.Build()
	require.NoError(t, err)

	topo := compiled.GetTopology()

	assert.Len(t, topo.Nodes, 2)
	assert.Equal(t, []string{"a"}, topo.EntryPoints)
	assert.Equal(t, []string{"b"}, topo.ExitPoints)

	// Check edges include START -> a and a -> b and b -> END
	edgeCount := 0
	for _, edge := range topo.Edges {
		if edge.From == "__start__" && edge.To == "a" {
			edgeCount++
		}
		if edge.From == "a" && edge.To == "b" {
			edgeCount++
		}
		if edge.From == "b" && edge.To == graph.END {
			edgeCount++
		}
	}
	assert.Equal(t, 3, edgeCount)
}

func TestGetMetrics(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("a", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("b", "c")
	}, "b", "c")
	g.Node("b", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("d")
	}, "d")
	g.Node("c", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("d")
	}, "d")
	g.Node("d", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	compiled, err := g.Build()
	require.NoError(t, err)

	metrics := compiled.GetMetrics()

	assert.Equal(t, 4, metrics.TotalNodes)
	// Edges: START->a, a->b, a->c, b->d, c->d, d->END = 6
	assert.Equal(t, 6, metrics.TotalEdges)
	assert.Equal(t, 2, metrics.MaxFanOut) // node "a" has 2 outgoing
	assert.Equal(t, 2, metrics.MaxFanIn)  // node "d" has 2 incoming
	assert.Greater(t, metrics.AverageFanOut, 0.0)
	assert.Greater(t, metrics.AverageFanIn, 0.0)
	assert.Greater(t, metrics.CyclomaticComplexity, 0)

	assert.Equal(t, 1, metrics.NodesByType["entry"])
	assert.Equal(t, 1, metrics.NodesByType["terminal"])
	assert.Equal(t, 2, metrics.NodesByType["standard"])
}

func TestGetMetricsEmptyGraph(t *testing.T) {
	g := graph.New()
	compiled, err := g.Build(graph.WithoutValidation())
	require.NoError(t, err)

	metrics := compiled.GetMetrics()

	assert.Equal(t, 0, metrics.TotalNodes)
	assert.Equal(t, 0, metrics.TotalEdges)
	assert.Equal(t, 0.0, metrics.AverageFanOut)
	assert.Equal(t, 0.0, metrics.AverageFanIn)
}

func TestMermaidFlowchart(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("a", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To("b", "c")
	}, "b", "c")
	g.Node("b", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Node("c", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	compiled, err := g.Build()
	require.NoError(t, err)

	t.Run("default direction", func(t *testing.T) {
		mermaid := compiled.MermaidFlowchart("")
		assert.Contains(t, mermaid, "graph TD")
		assert.Contains(t, mermaid, "__start__([START])")
		assert.Contains(t, mermaid, "__end__([END])")
		assert.Contains(t, mermaid, "__start__ --> a")
	})

	t.Run("left-right direction", func(t *testing.T) {
		mermaid := compiled.MermaidFlowchart("LR")
		assert.Contains(t, mermaid, "graph LR")
	})

	t.Run("branching node uses diamond", func(t *testing.T) {
		mermaid := compiled.MermaidFlowchart("TD")
		// Node "a" has multiple targets, should use diamond shape
		assert.Contains(t, mermaid, "a{a}")
	})
}

func TestTopologyWithMultipleEntryPoints(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("a", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Node("b", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a", "b")

	compiled, err := g.Build()
	require.NoError(t, err)

	topo := compiled.GetTopology()
	assert.ElementsMatch(t, []string{"a", "b"}, topo.EntryPoints)
	assert.ElementsMatch(t, []string{"a", "b"}, topo.ExitPoints)
}
