package middleware

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ConditionalMiddleware applies middleware only when the condition is met.
//
// Example:
//
//	// Only log expensive nodes
//	graph.WithMiddleware(graphmw.ConditionalMiddleware(
//	    func(scope graph.Scope[message.Message]) bool {
//	        return scope.NodeName() == "expensive_node"
//	    },
//	    graphmw.LoggingMiddleware[message.Message](logger),
//	))
func ConditionalMiddleware[O any](condition func(scope graph.Scope[O]) bool, mw graph.Middleware[O]) graph.Middleware[O] {
	return func(next graph.NodeFunc[O]) graph.NodeFunc[O] {
		wrapped := mw(next)
		return func(ctx context.Context, scope graph.Scope[O]) (*graph.Command, error) {
			if condition(scope) {
				return wrapped(ctx, scope)
			}
			return next(ctx, scope)
		}
	}
}

// NodeMiddleware applies middleware only to specific nodes.
//
// Example:
//
//	graph.WithMiddleware(graphmw.NodeMiddleware(
//	    []string{"slow_node", "external_api"},
//	    graphmw.TimingMiddleware[message.Message](recordTiming),
//	))
func NodeMiddleware[O any](nodeNames []string, mw graph.Middleware[O]) graph.Middleware[O] {
	nodeSet := make(map[string]bool, len(nodeNames))
	for _, name := range nodeNames {
		nodeSet[name] = true
	}

	return ConditionalMiddleware(
		func(scope graph.Scope[O]) bool {
			return nodeSet[scope.NodeName()]
		},
		mw,
	)
}
