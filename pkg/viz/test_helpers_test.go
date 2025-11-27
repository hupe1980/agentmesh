package viz

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// mockRunnable is a test helper that implements viz.Runnable
type mockRunnable struct {
	nodes         []string
	topology      *graph.Topology
	mermaid       string
	executeFunc   func(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error]
	executeCalled bool
}

func (m *mockRunnable) Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error] {
	m.executeCalled = true
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input, opts...)
	}
	return func(yield func(any, error) bool) {
		yield("mock-output", nil)
	}
}

func (m *mockRunnable) GetNodes() []string {
	if m.nodes != nil {
		return m.nodes
	}
	return []string{"node1", "node2"}
}

func (m *mockRunnable) GetTopology() *graph.Topology {
	return m.topology
}

func (m *mockRunnable) MermaidFlowchart(direction string) string {
	if m.mermaid != "" {
		return m.mermaid
	}
	return "graph LR\n  A-->B"
}
