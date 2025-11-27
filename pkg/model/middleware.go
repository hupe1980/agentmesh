package model

import (
	"context"
	"iter"
)

// Middleware intercepts and extends model execution.
type Middleware interface {
	Wrap(next Executor) Executor
}

// MiddlewareFunc is a function adapter for Middleware.
type MiddlewareFunc func(next Executor) Executor

// Wrap implements the Middleware interface.
func (f MiddlewareFunc) Wrap(next Executor) Executor {
	return f(next)
}

// Chain applies multiple middleware to an executor in order.
func Chain(executor Executor, middleware ...Middleware) Executor {
	for i := len(middleware) - 1; i >= 0; i-- {
		executor = middleware[i].Wrap(executor)
	}
	return executor
}

// ExecutorWrapper wraps a function as an Executor.
type ExecutorWrapper struct {
	generateFunc func(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}

// Generate implements the Executor interface.
func (w *ExecutorWrapper) Generate(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return w.generateFunc(ctx, req)
}

// WrapFunc creates an executor from a function.
func WrapFunc(fn func(ctx context.Context, req *Request) iter.Seq2[*Response, error]) Executor {
	return &ExecutorWrapper{generateFunc: fn}
}
