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
//	graph.WithMiddleware(graphmw.TimingMiddleware[message.Message](func(node string, d time.Duration) {
//	    metrics.RecordLatency(node, d)
//	}))
func TimingMiddleware[O any](onComplete func(nodeName string, duration time.Duration)) graph.Middleware[O] {
	return func(next graph.NodeFunc[O]) graph.NodeFunc[O] {
		return func(ctx context.Context, scope graph.Scope[O]) (*graph.Command, error) {
			start := time.Now()
			cmd, err := next(ctx, scope)
			if onComplete != nil {
				onComplete(scope.NodeName(), time.Since(start))
			}
			return cmd, err
		}
	}
}
