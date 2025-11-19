package graph

import (
	"context"
	"iter"
)

// Executor defines a strategy for executing a compiled graph.
// Generic over input and output types to provide type-safe execution.
//
// Implementers:
//   - PregelExecutor: BSP-based parallel execution (default)
//   - SequentialExecutor: Simple sequential execution for debugging
//
// Type parameters:
//   - I: Input type accepted by the executor
//   - O: Output type produced by the executor
type Executor[I, O any] interface {
	// Run executes the compiled graph with the given input.
	// Returns an iterator that yields outputs as execution progresses.
	//
	// The iterator approach allows:
	//   - Streaming results as they become available
	//   - Early termination on errors
	//   - Resource cleanup via context cancellation
	//
	// Uses RunOptions from runnable.go for configuration.
	Run(ctx context.Context, compiled *Compiled[I, O], input I, opts ...RunOption) iter.Seq2[O, error]
}
