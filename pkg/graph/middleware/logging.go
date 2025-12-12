package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// LoggingMiddleware creates middleware that logs node execution.
//
// Example:
//
//	graph.WithMiddleware(graphmw.LoggingMiddleware[message.Message](slog.Default()))
func LoggingMiddleware[O any](logger *slog.Logger) graph.Middleware[O] {
	return func(next graph.NodeFunc[O]) graph.NodeFunc[O] {
		return func(ctx context.Context, scope graph.Scope[O]) (*graph.Command, error) {
			nodeName := scope.NodeName()
			logger.DebugContext(ctx, "node started", "node", nodeName)

			start := time.Now()
			cmd, err := next(ctx, scope)
			duration := time.Since(start)

			if err != nil {
				logger.ErrorContext(ctx, "node failed",
					"node", nodeName,
					"duration", duration,
					"error", err,
				)
			} else {
				logger.DebugContext(ctx, "node completed",
					"node", nodeName,
					"duration", duration,
				)
			}

			return cmd, err
		}
	}
}
