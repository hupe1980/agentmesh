package middleware

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/viz"
)

// VizMiddleware integrates graph execution with the visualization server.
// It subscribes a viz event handler to the graph event bus.
type VizMiddleware[I, O any] struct {
	server *viz.Server
	runID  string
}

// NewVizMiddleware creates a new visualization middleware.
// The runID identifies this execution run in the visualization server.
func NewVizMiddleware[I, O any](server *viz.Server, runID string) *VizMiddleware[I, O] {
	return &VizMiddleware[I, O]{
		server: server,
		runID:  runID,
	}
}

// Wrap wraps the graph executor with visualization integration.
func (m *VizMiddleware[I, O]) Wrap(next graph.Executor[I, O]) graph.Executor[I, O] {
	return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
		// Create and subscribe viz event handler
		handler := viz.NewGraphEventHandler(m.server, m.runID)
		ctx = handler.SubscribeToGraph(ctx)

		// Execute with visualization enabled
		return next.Run(ctx, compiled, input, opts...)
	})
}
