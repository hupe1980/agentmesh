package graph

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// nodeNameKey is the context key for node name.
type nodeNameKey struct{}

// WithNodeName attaches the current node name to the context.
// This is called automatically by the executor before running each node.
func WithNodeName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, nodeNameKey{}, name)
}

// NodeNameFromContext retrieves the current node name from context.
// Returns empty string if not in a node execution context.
func NodeNameFromContext(ctx context.Context) string {
	if name, ok := ctx.Value(nodeNameKey{}).(string); ok {
		return name
	}
	return ""
}

// Chain combines multiple middleware into one.
// Middleware are applied in order, so the first middleware is the outermost layer.
//
// Example:
//
//	combined := graph.Chain(
//	    LoggingMiddleware(logger),
//	    TracingMiddleware(),
//	    MetricsMiddleware(),
//	)
//	graph.WithMiddleware(combined)
//
// This produces: logging(tracing(metrics(node)))
// Execution flows: logging → tracing → metrics → node
func Chain(middleware ...Middleware) Middleware {
	return func(next NodeFunc) NodeFunc {
		// Apply in reverse order so first middleware is outermost
		for i := len(middleware) - 1; i >= 0; i-- {
			next = middleware[i](next)
		}
		return next
	}
}

// LoggingMiddleware creates middleware that logs node execution.
//
// Example:
//
//	graph.WithMiddleware(graph.LoggingMiddleware(slog.Default()))
func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next NodeFunc) NodeFunc {
		return func(ctx context.Context, view View) (*Command, error) {
			nodeName := NodeNameFromContext(ctx)
			logger.DebugContext(ctx, "node started", "node", nodeName)

			start := time.Now()
			cmd, err := next(ctx, view)
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

// TimingMiddleware creates middleware that tracks execution time.
// The duration is available via the callback function.
//
// Example:
//
//	graph.WithMiddleware(graph.TimingMiddleware(func(node string, d time.Duration) {
//	    metrics.RecordLatency(node, d)
//	}))
func TimingMiddleware(onComplete func(nodeName string, duration time.Duration)) Middleware {
	return func(next NodeFunc) NodeFunc {
		return func(ctx context.Context, view View) (*Command, error) {
			start := time.Now()
			cmd, err := next(ctx, view)
			if onComplete != nil {
				onComplete(NodeNameFromContext(ctx), time.Since(start))
			}
			return cmd, err
		}
	}
}

// RecoveryMiddleware creates middleware that recovers from panics.
// It converts panics into errors rather than crashing the entire graph.
//
// Example:
//
//	graph.WithMiddleware(graph.RecoveryMiddleware(func(node string, recovered any) {
//	    logger.Error("panic recovered", "node", node, "panic", recovered)
//	}))
func RecoveryMiddleware(onPanic func(nodeName string, recovered any)) Middleware {
	return func(next NodeFunc) NodeFunc {
		return func(ctx context.Context, view View) (cmd *Command, err error) {
			defer func() {
				if r := recover(); r != nil {
					if onPanic != nil {
						onPanic(NodeNameFromContext(ctx), r)
					}
					// Convert panic to error
					if e, ok := r.(error); ok {
						err = e
					} else {
						err = fmt.Errorf("panic in node %s: %v", NodeNameFromContext(ctx), r)
					}
				}
			}()
			return next(ctx, view)
		}
	}
}

// ConditionalMiddleware applies middleware only when the condition is met.
//
// Example:
//
//	// Only log expensive nodes
//	graph.WithMiddleware(graph.ConditionalMiddleware(
//	    func(ctx context.Context) bool {
//	        return NodeNameFromContext(ctx) == "expensive_node"
//	    },
//	    graph.LoggingMiddleware(logger),
//	))
func ConditionalMiddleware(condition func(ctx context.Context) bool, mw Middleware) Middleware {
	return func(next NodeFunc) NodeFunc {
		wrapped := mw(next)
		return func(ctx context.Context, view View) (*Command, error) {
			if condition(ctx) {
				return wrapped(ctx, view)
			}
			return next(ctx, view)
		}
	}
}

// NodeMiddleware applies middleware only to specific nodes.
//
// Example:
//
//	graph.WithMiddleware(graph.NodeMiddleware(
//	    []string{"slow_node", "external_api"},
//	    graph.TimingMiddleware(recordTiming),
//	))
func NodeMiddleware(nodeNames []string, mw Middleware) Middleware {
	nodeSet := make(map[string]bool, len(nodeNames))
	for _, name := range nodeNames {
		nodeSet[name] = true
	}

	return ConditionalMiddleware(
		func(ctx context.Context) bool {
			return nodeSet[NodeNameFromContext(ctx)]
		},
		mw,
	)
}
