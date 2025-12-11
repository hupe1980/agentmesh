package middleware

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/viz"
)

// VizMiddleware integrates graph node execution with the visualization server.
// It subscribes a viz event handler to the graph event bus.
type VizMiddleware struct {
	server *viz.Server
	runID  string
}

// NewVizMiddleware creates a new visualization middleware.
// The runID identifies this execution run in the visualization server.
func NewVizMiddleware(server *viz.Server, runID string) *VizMiddleware {
	return &VizMiddleware{
		server: server,
		runID:  runID,
	}
}

// Middleware returns a generic node middleware that integrates with the visualization server.
// Apply this to nodes that should report their execution to the viz server.
// The type parameter O must match the graph's output type.
//
// Example:
//
//	g := graph.New[[]message.Message, message.Message](...)
//	g.WithMiddleware(middleware.Middleware[message.Message](server, runID))
func Middleware[O any](server *viz.Server, runID string) graph.Middleware[O] {
	return func(next graph.NodeFunc[O]) graph.NodeFunc[O] {
		return func(ctx context.Context, scope graph.Scope[O]) (*graph.Command, error) {
			// Create and subscribe viz event handler
			handler := viz.NewGraphEventHandler(server, runID)
			ctx = handler.SubscribeToGraph(ctx)

			// Execute the node
			return next(ctx, scope)
		}
	}
}
