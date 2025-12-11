package graph

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Chain combines multiple middleware into one.
// Middleware are applied in order, so the first middleware is the outermost layer.
//
// Example:
//
//	combined := graph.Chain(
//	    LoggingMiddleware[message.Message](logger),
//	    TracingMiddleware[message.Message](),
//	    MetricsMiddleware[message.Message](),
//	)
//	graph.WithMiddleware(combined)
//
// This produces: logging(tracing(metrics(node)))
// Execution flows: logging → tracing → metrics → node
func Chain[O any](middleware ...Middleware[O]) Middleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
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
//	graph.WithMiddleware(graph.LoggingMiddleware[message.Message](slog.Default()))
func LoggingMiddleware[O any](logger *slog.Logger) Middleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
		return func(ctx context.Context, scope Scope[O]) (*Command, error) {
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

// TimingMiddleware creates middleware that tracks execution time.
// The duration is available via the callback function.
//
// Example:
//
//	graph.WithMiddleware(graph.TimingMiddleware[message.Message](func(node string, d time.Duration) {
//	    metrics.RecordLatency(node, d)
//	}))
func TimingMiddleware[O any](onComplete func(nodeName string, duration time.Duration)) Middleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
		return func(ctx context.Context, scope Scope[O]) (*Command, error) {
			start := time.Now()
			cmd, err := next(ctx, scope)
			if onComplete != nil {
				onComplete(scope.NodeName(), time.Since(start))
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
//	graph.WithMiddleware(graph.RecoveryMiddleware[message.Message](func(node string, recovered any) {
//	    logger.Error("panic recovered", "node", node, "panic", recovered)
//	}))
func RecoveryMiddleware[O any](onPanic func(nodeName string, recovered any)) Middleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
		return func(ctx context.Context, scope Scope[O]) (cmd *Command, err error) {
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

// ConditionalMiddleware applies middleware only when the condition is met.
//
// Example:
//
//	// Only log expensive nodes
//	graph.WithMiddleware(graph.ConditionalMiddleware(
//	    func(scope graph.Scope[message.Message]) bool {
//	        return scope.NodeName() == "expensive_node"
//	    },
//	    graph.LoggingMiddleware[message.Message](logger),
//	))
func ConditionalMiddleware[O any](condition func(scope Scope[O]) bool, mw Middleware[O]) Middleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
		wrapped := mw(next)
		return func(ctx context.Context, scope Scope[O]) (*Command, error) {
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
//	graph.WithMiddleware(graph.NodeMiddleware(
//	    []string{"slow_node", "external_api"},
//	    graph.TimingMiddleware[message.Message](recordTiming),
//	))
func NodeMiddleware[O any](nodeNames []string, mw Middleware[O]) Middleware[O] {
	nodeSet := make(map[string]bool, len(nodeNames))
	for _, name := range nodeNames {
		nodeSet[name] = true
	}

	return ConditionalMiddleware(
		func(scope Scope[O]) bool {
			return nodeSet[scope.NodeName()]
		},
		mw,
	)
}
