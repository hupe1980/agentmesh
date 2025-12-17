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
//	graph.WithNodeMiddleware(graphmw.LoggingMiddleware(slog.Default()))
func LoggingMiddleware(logger *slog.Logger) graph.NodeMiddleware {
	return func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
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
