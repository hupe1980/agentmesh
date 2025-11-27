package graph

import (
	"context"
	"iter"
)

// Middleware intercepts and extends graph execution.
// Middleware can add cross-cutting concerns like tracing, metrics, caching, etc.
// without modifying the core graph execution logic.
//
// Example:
//
//	type LoggingMiddleware struct {
//	    logger *log.Logger
//	}
//
//	func (m *LoggingMiddleware) Wrap(next graph.Executor[I, O]) graph.Executor[I, O] {
//	    return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
//	        m.logger.Println("Starting execution")
//	        return next.Run(ctx, compiled, input, opts...)
//	    })
//	}
type Middleware[I, O any] interface {
	// Wrap takes the next executor in the chain and returns a wrapped version.
	// The wrapped executor should call next.Run() to continue the chain.
	Wrap(next Executor[I, O]) Executor[I, O]
}

// MiddlewareFunc is a function adapter for Middleware.
// It allows using functions as middleware without defining a type.
//
// Example:
//
//	loggingMiddleware := graph.MiddlewareFunc[Input, Output](func(next graph.Executor[Input, Output]) graph.Executor[Input, Output] {
//	    return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[Input, Output], input Input, opts ...graph.RunOption) iter.Seq2[Output, error] {
//	        log.Println("Starting execution")
//	        return next.Run(ctx, compiled, input, opts...)
//	    })
//	})
type MiddlewareFunc[I, O any] func(next Executor[I, O]) Executor[I, O]

// Wrap implements the Middleware interface.
func (f MiddlewareFunc[I, O]) Wrap(next Executor[I, O]) Executor[I, O] {
	return f(next)
}

// Chain applies multiple middleware to an executor in order.
// Middleware are applied in the order given, so the first middleware
// in the list is the outermost layer.
//
// Example:
//
//	executor := graph.Chain(
//	    graph.NewMessagePregelExecutor(),
//	    middleware.NewTraceMiddleware(),
//	    middleware.NewMetricsMiddleware(),
//	    middleware.NewCacheMiddleware(),
//	)
//
// This produces: trace(metrics(cache(executor)))
// Execution flows: trace → metrics → cache → executor
func Chain[I, O any](executor Executor[I, O], middleware ...Middleware[I, O]) Executor[I, O] {
	// Apply middleware in reverse order so the first middleware is outermost
	for i := len(middleware) - 1; i >= 0; i-- {
		executor = middleware[i].Wrap(executor)
	}
	return executor
}

// ExecutorWrapper wraps a function as an Executor.
// This is useful for creating ad-hoc executors or for middleware implementations.
type ExecutorWrapper[I, O any] struct {
	runFunc func(ctx context.Context, compiled *Compiled[I, O], input I, opts ...RunOption) iter.Seq2[O, error]
}

// Run implements the Executor interface.
func (w *ExecutorWrapper[I, O]) Run(ctx context.Context, compiled *Compiled[I, O], input I, opts ...RunOption) iter.Seq2[O, error] {
	return w.runFunc(ctx, compiled, input, opts...)
}

// WrapFunc creates an executor from a function.
// This is a convenience function for middleware implementations.
//
// Example:
//
//	return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
//	    // Pre-processing
//	    results := next.Run(ctx, compiled, input, opts...)
//	    // Post-processing
//	    return results
//	})
func WrapFunc[I, O any](fn func(ctx context.Context, compiled *Compiled[I, O], input I, opts ...RunOption) iter.Seq2[O, error]) Executor[I, O] {
	return &ExecutorWrapper[I, O]{runFunc: fn}
}
