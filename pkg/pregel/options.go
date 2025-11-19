package pregel

import (
	"context"
	"runtime"
	"time"
)

// QuotaConfig defines resource limits for graph execution.
type QuotaConfig struct {
	// MaxMemoryBytes limits heap memory usage (0 = unlimited).
	// When exceeded, triggers GC and fails execution if still over limit.
	// Recommended: 512MB to 2GB depending on graph size.
	MaxMemoryBytes uint64

	// MaxGoroutines limits concurrent goroutines (0 = unlimited).
	// Applies backpressure when limit is reached.
	// Recommended: 100-1000 depending on system capacity.
	MaxGoroutines int

	// MaxExecutionTime limits total execution duration (0 = unlimited).
	// Prevents infinite loops and time-based DoS.
	// Recommended: 5-30 minutes for typical workflows.
	MaxExecutionTime time.Duration
}

// RuntimeOptions configures runtime behaviour for a Pregel computation over
// state type S with messages of type M.
type RuntimeOptions[S any, M any] struct {
	// MaxWorkers bounds the number of concurrent vertex executions per superstep.
	// Defaults to runtime.NumCPU().
	MaxWorkers int

	// InitialSuperstep seeds the superstep counter before Run executes. Useful
	// when resuming from a persisted snapshot.
	InitialSuperstep int64

	// MaxIterations limits the number of iterations (supersteps) the runtime will
	// execute before returning ErrMaxIterationsExceeded. A value <= 0 means
	// unlimited. This prevents infinite loops in cyclic graphs.
	MaxIterations int

	// Aggregators defines the named global reductions that vertices can
	// contribute to during execution.
	Aggregators map[string]Aggregator

	// Combiner optionally merges multiple messages for the same target within a
	// superstep.
	Combiner Combiner[M]

	// MaxMailboxSize limits the number of messages that can accumulate in any
	// single vertex's mailbox. When this limit is reached, new messages are
	// rejected with ErrMailboxFull. A value <= 0 means unlimited (default).
	// Recommended: Set this to prevent memory exhaustion in high-throughput scenarios.
	MaxMailboxSize int

	// MessageBus provides pluggable message delivery backend. If nil, defaults
	// to InMemoryMessageBus with MaxMailboxSize and Combiner settings.
	// Use this to enable distributed execution (Redis, gRPC, etc.)
	MessageBus MessageBus[M]

	// OnSuperstepStart is called before each superstep begins.
	// The callback receives the execution context and superstep number. Useful for BSP snapshots.
	OnSuperstepStart func(ctx context.Context, superstep int64) error

	// OnSuperstepComplete is called after each superstep completes successfully.
	// The callback receives the execution context and superstep number. Useful for checkpointing.
	OnSuperstepComplete func(ctx context.Context, superstep int64) error

	// VertexTimeout sets the maximum execution time for a single vertex.
	// If a vertex takes longer than this duration, its context is cancelled and
	// execution returns an error. This prevents a single slow/hanging node from
	// blocking the entire superstep. A value <= 0 means no timeout (default).
	// Recommended: 30s for typical workflows, adjust based on expected node complexity.
	VertexTimeout time.Duration

	// QuotaConfig defines resource quotas (memory, goroutines, time) to prevent
	// resource exhaustion during graph execution. If nil, no quotas are enforced.
	// Use this to prevent runaway memory usage, goroutine leaks, and time-based DoS.
	QuotaConfig *QuotaConfig
}

// RuntimeOption mutates runtime options.
type RuntimeOption[S any, M any] func(*RuntimeOptions[S, M])

// WithMaxWorkers limits worker count.
func WithMaxWorkers[S any, M any](maxWorkers int) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		if maxWorkers > 0 {
			o.MaxWorkers = maxWorkers
		}
	}
}

// WithInitialSuperstep seeds the runtime superstep counter, enabling resumed
// executions to maintain monotonic superstep values.
func WithInitialSuperstep[S any, M any](superstep int64) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.InitialSuperstep = superstep
	}
}

// WithAggregators installs one or more global aggregators. A defensive copy of
// the provided map is stored so callers can reuse their input map safely.
func WithAggregators[S any, M any](aggregators map[string]Aggregator) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		if len(aggregators) == 0 {
			o.Aggregators = nil
			return
		}
		aggCopy := make(map[string]Aggregator, len(aggregators))
		for name, agg := range aggregators {
			if name == "" || agg == nil {
				continue
			}
			aggCopy[name] = agg
		}
		o.Aggregators = aggCopy
	}
}

// WithCombiner installs a message combiner used during delivery. Passing nil
// removes any existing combiner.
func WithCombiner[S any, M any](combiner Combiner[M]) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.Combiner = combiner
	}
}

// WithMaxIterations sets the maximum number of iterations (supersteps) allowed
// before terminating with ErrMaxIterationsExceeded. A value <= 0 means unlimited.
// This is critical for preventing infinite loops in cyclic graphs.
func WithMaxIterations[S any, M any](n int) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.MaxIterations = n
	}
}

// WithMaxMailboxSize sets the maximum number of messages allowed in any single
// vertex's mailbox. When exceeded, message delivery returns ErrMailboxFull.
// A value <= 0 means unlimited (default).
//
// Use this to prevent memory exhaustion in high-throughput scenarios:
//   - Small graphs (< 100 nodes): 10,000 messages per node
//   - Medium graphs (100-1000 nodes): 1,000 messages per node
//   - Large graphs (> 1000 nodes): 100-500 messages per node
func WithMaxMailboxSize[S any, M any](size int) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.MaxMailboxSize = size
	}
}

// WithOnSuperstepStart sets a callback that is invoked before each superstep
// begins. Useful for creating BSP-compliant state snapshots.
func WithOnSuperstepStart[S any, M any](callback func(ctx context.Context, superstep int64) error) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.OnSuperstepStart = callback
	}
}

// WithOnSuperstepComplete sets a callback that is invoked after each superstep
// completes successfully. Useful for checkpointing or progress monitoring.
func WithOnSuperstepComplete[S any, M any](callback func(ctx context.Context, superstep int64) error) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.OnSuperstepComplete = callback
	}
}

// WithMessageBus sets a custom message delivery backend, enabling distributed
// execution or alternative storage strategies.
//
// Examples:
//   - Redis-backed bus for multi-node deployments
//   - Persisted bus for replay debugging
//   - gRPC bus for cross-process coordination
//
// If not provided, defaults to InMemoryMessageBus.
func WithMessageBus[S any, M any](bus MessageBus[M]) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.MessageBus = bus
	}
}

// WithVertexTimeout sets the maximum execution time for a single vertex.
// If a vertex exceeds this duration, its execution is cancelled with a context
// timeout error. This prevents hanging vertices from blocking superstep progress.
//
// Recommended values:
//   - Fast operations (< 1s): 5 * time.Second
//   - Normal workflows: 30 * time.Second (default recommendation)
//   - Long-running tasks: 5 * time.Minute
//   - No timeout: 0 (default, not recommended for production)
func WithVertexTimeout[S any, M any](timeout time.Duration) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.VertexTimeout = timeout
	}
}

// WithQuotaConfig sets resource quotas to prevent memory exhaustion,
// goroutine leaks, and runaway execution time.
//
// Recommended for production deployments to enforce resource limits.
//
// Example:
//
//	runtime.Run(ctx, state, graph, pregel.WithQuotaConfig(&pregel.QuotaConfig{
//	    MaxMemoryBytes:   1024 * 1024 * 1024, // 1 GB
//	    MaxGoroutines:    500,
//	    MaxExecutionTime: 10 * time.Minute,
//	}))
func WithQuotaConfig[S any, M any](config *QuotaConfig) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.QuotaConfig = config
	}
}

func defaultRuntimeOptions[S any, M any]() RuntimeOptions[S, M] {
	return RuntimeOptions[S, M]{
		MaxWorkers:       runtime.NumCPU(),
		InitialSuperstep: 0,
	}
}

// RuntimeStats collects execution metrics recorded during the run. Each field
// represents the total count observed so far.
type RuntimeStats struct {
	Supersteps int64
	Vertices   int64
	Messages   int64
}
