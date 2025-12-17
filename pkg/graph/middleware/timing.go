package middleware

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// TimingMiddleware creates middleware that tracks execution time.
// The duration is available via the callback function.
//
// Example:
//
//	graph.WithNodeMiddleware(graphmw.TimingMiddleware(func(node string, d time.Duration) {
//	    metrics.RecordLatency(node, d)
//	}))
func TimingMiddleware(onComplete func(nodeName string, duration time.Duration)) graph.NodeMiddleware {
	return func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			start := time.Now()
			cmd, err := next(ctx, scope)
			if onComplete != nil {
				onComplete(scope.NodeName(), time.Since(start))
			}
			return cmd, err
		}
	}
}
