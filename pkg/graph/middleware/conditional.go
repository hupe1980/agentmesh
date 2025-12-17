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
//	graph.WithNodeMiddleware(graphmw.ConditionalMiddleware(
//	    func(scope graph.Scope) bool {
//	        return scope.NodeName() == "expensive_node"
//	    },
//	    graphmw.LoggingMiddleware(logger),
//	))
func ConditionalMiddleware(condition func(scope graph.Scope) bool, mw graph.NodeMiddleware) graph.NodeMiddleware {
	return func(next graph.NodeFunc) graph.NodeFunc {
		wrapped := mw(next)
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			if condition(scope) {
				return wrapped(ctx, scope)
			}
			return next(ctx, scope)
		}
	}
}

// NodeNameMiddleware applies middleware only to specific nodes.
//
// Example:
//
//	graph.WithNodeMiddleware(graphmw.NodeNameMiddleware(
//	    []string{"slow_node", "external_api"},
//	    graphmw.TimingMiddleware(recordTiming),
//	))
func NodeNameMiddleware(nodeNames []string, mw graph.NodeMiddleware) graph.NodeMiddleware {
	nodeSet := make(map[string]bool, len(nodeNames))
	for _, name := range nodeNames {
		nodeSet[name] = true
	}

	return ConditionalMiddleware(
		func(scope graph.Scope) bool {
			return nodeSet[scope.NodeName()]
		},
		mw,
	)
}
