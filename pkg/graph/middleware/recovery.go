package middleware

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// RecoveryMiddleware creates middleware that recovers from panics.
// It converts panics into errors rather than crashing the entire graph.
//
// Example:
//
//	graph.WithMiddleware(graphmw.RecoveryMiddleware[message.Message](func(node string, recovered any) {
//	    logger.Error("panic recovered", "node", node, "panic", recovered)
//	}))
func RecoveryMiddleware[O any](onPanic func(nodeName string, recovered any)) graph.Middleware[O] {
	return func(next graph.NodeFunc[O]) graph.NodeFunc[O] {
		return func(ctx context.Context, scope graph.Scope[O]) (cmd *graph.Command, err error) {
			nodeName := scope.NodeName()
			defer func() {
				if r := recover(); r != nil {
					if onPanic != nil {
						onPanic(nodeName, r)
					}
					// Convert panic to error
					if e, ok := r.(error); ok {
						err = e
					} else {
						err = fmt.Errorf("panic in node %s: %v", nodeName, r)
					}
				}
			}()
			return next(ctx, scope)
		}
	}
}
