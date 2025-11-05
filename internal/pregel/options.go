package pregel

import "runtime"

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

	// OnSuperstepComplete is called after each superstep completes successfully.
	// The callback receives the superstep number. Useful for checkpointing.
	OnSuperstepComplete func(superstep int64)
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

// WithOnSuperstepComplete sets a callback that is invoked after each superstep
// completes successfully. Useful for checkpointing or progress monitoring.
func WithOnSuperstepComplete[S any, M any](callback func(superstep int64)) RuntimeOption[S, M] {
	return func(o *RuntimeOptions[S, M]) {
		o.OnSuperstepComplete = callback
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
