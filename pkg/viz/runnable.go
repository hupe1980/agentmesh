package viz

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Runnable represents an executable graph that can be registered with the visualization server.
// This is the primary interface for integrating graphs with the viz server.
type Runnable interface {
	// Execute runs the graph with untyped HTTP input and returns an iterator of untyped outputs.
	// The visualization server calls this method to execute registered graphs.
	Execute(ctx context.Context, input map[string]any, opts ...graph.RunOption) iter.Seq2[any, error]

	// Introspection methods for visualization
	GetNodes() []string
	GetTopology() *graph.Topology
	MermaidFlowchart(direction string) string
}

// Ensure graph.CompiledGraph implements the necessary introspection methods at compile time
var _ interface {
	GetNodes() []string
	GetTopology() *graph.Topology
	MermaidFlowchart(string) string
} = (*graph.CompiledGraph[any, any])(nil)
