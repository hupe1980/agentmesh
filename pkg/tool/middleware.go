package tool

import (
	"context"
)

// Middleware intercepts and extends tool execution.
// Middleware can add cross-cutting concerns like timeouts, circuit breakers, caching, etc.
// without modifying the tool executor implementation.
//
// Example:
//
//	type TimeoutMiddleware struct {
//	    timeout time.Duration
//	}
//
//	func (m *TimeoutMiddleware) Wrap(next tool.Executor) tool.Executor {
//	    return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
//	        ctx, cancel := context.WithTimeout(ctx, m.timeout)
//	        defer cancel()
//	        return next.Execute(ctx, calls)
//	    })
//	}
type Middleware interface {
	// Wrap takes the next executor in the chain and returns a wrapped version.
	// The wrapped executor should call next.Execute() to continue the chain.
	Wrap(next Executor) Executor
}

// MiddlewareFunc is a function adapter for Middleware.
// It allows using functions as middleware without defining a type.
//
// Example:
//
//	loggingMiddleware := tool.MiddlewareFunc(func(next tool.Executor) tool.Executor {
//	    return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
//	        log.Printf("Executing %d tools", len(calls))
//	        return next.Execute(ctx, calls)
//	    })
//	})
type MiddlewareFunc func(next Executor) Executor

// Wrap implements the Middleware interface.
func (f MiddlewareFunc) Wrap(next Executor) Executor {
	return f(next)
}

// Chain applies multiple middleware to an executor in order.
// Middleware are applied in the order given, so the first middleware
// in the list is the outermost layer.
//
// Example:
//
//	executor := tool.Chain(
//	    tool.NewSequentialExecutor(registry),
//	    middleware.NewCacheMiddleware(),
//	    middleware.NewTimeoutMiddleware(30*time.Second),
//	    middleware.NewCircuitBreakerMiddleware(5, time.Minute),
//	)
//
// This produces: cache(timeout(circuitBreaker(executor)))
// Execution flows: cache → timeout → circuitBreaker → executor
func Chain(executor Executor, middleware ...Middleware) Executor {
	// Apply middleware in reverse order so the first middleware is outermost
	for i := len(middleware) - 1; i >= 0; i-- {
		executor = middleware[i].Wrap(executor)
	}
	return executor
}

// ExecutorWrapper wraps a function as an Executor.
// This is useful for creating ad-hoc executors or for middleware implementations.
type ExecutorWrapper struct {
	executeFunc func(ctx context.Context, calls []Call) ([]ExecutionResult, error)
}

// Execute implements the Executor interface.
func (w *ExecutorWrapper) Execute(ctx context.Context, calls []Call) ([]ExecutionResult, error) {
	return w.executeFunc(ctx, calls)
}

// WrapFunc creates an executor from a function.
// This is a convenience function for middleware implementations.
//
// Example:
//
//	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
//	        // Pre-processing
//	        start := time.Now()
//	        results, err := next.Execute(ctx, calls)
//	        // Post-processing
//	        log.Printf("Tool execution took: %v", time.Since(start))
//	        return results, err
//	})
func WrapFunc(fn func(ctx context.Context, calls []Call) ([]ExecutionResult, error)) Executor {
	return &ExecutorWrapper{executeFunc: fn}
}
