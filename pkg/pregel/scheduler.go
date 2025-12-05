package pregel

import (
	"context"
	"sort"
)

// Scheduler determines which vertices to execute and in what order during each superstep.
// Different implementations can customize execution order based on priorities, resources,
// or adaptive learning patterns.
//
// DESIGN NOTE:
// The scheduler operates within the BSP (Bulk Synchronous Parallel) execution model:
//   - Receives frontier (vertices with pending messages) at superstep start
//   - Returns ordered list of vertices to execute in parallel
//   - No scheduling state persists across supersteps (stateless between barriers)
//
// IMPLEMENTATION GUIDELINES:
//   - NextBatch() must be thread-safe (called once per superstep)
//   - Batch ordering affects execution order but not correctness (BSP semantics)
//   - Can inspect graph topology, state, and message counts for decisions
//   - RecordCompletion() is optional - used by adaptive schedulers for learning
type Scheduler interface {
	// NextBatch returns vertices ready for execution in the current superstep.
	// The returned slice determines execution order: vertices at the start of
	// the slice are processed first by the worker pool.
	//
	// Parameters:
	//   - ctx: Context for cancellation and dependency injection
	//   - info: Information about the current scheduling state
	//
	// Returns:
	//   - []string: Ordered list of vertex names to execute
	//   - error: Any error that prevents scheduling (e.g., resource unavailable)
	//
	// Thread-safety: Must be safe to call concurrently with RecordCompletion
	NextBatch(ctx context.Context, info SchedulerInfo) ([]string, error)

	// RecordCompletion notifies the scheduler that a vertex has completed execution.
	// Optional callback for adaptive schedulers that learn from execution patterns.
	//
	// Parameters:
	//   - ctx: Context for cancellation and dependency injection
	//   - vertex: Name of the completed vertex
	//   - info: Information about the completed execution
	//
	// Thread-safety: Called concurrently from multiple worker goroutines
	// Implementations must synchronize access to shared state
	RecordCompletion(ctx context.Context, vertex string, info CompletionInfo)
}

// SchedulerInfo provides context for scheduling decisions.
type SchedulerInfo struct {
	// Frontier contains vertices with pending messages for this superstep
	Frontier map[string]struct{}

	// Superstep is the current superstep number (starts at 1)
	Superstep int64

	// Graph provides access to topology and state
	Graph TopologyProvider

	// MessageCounts maps vertex names to number of pending messages
	// Useful for prioritizing vertices with more work
	MessageCounts map[string]int
}

// CompletionInfo provides feedback about completed vertex execution.
type CompletionInfo struct {
	// Duration is how long the vertex took to execute
	Duration int64 // nanoseconds

	// MessagesSent is the number of messages the vertex sent
	MessagesSent int

	// Error is any error returned by the vertex (nil if successful)
	Error error
}

// TopologyProvider provides read-only access to graph topology.
// Schedulers can inspect topology to make decisions but cannot modify it.
type TopologyProvider interface {
	// Outgoing returns the names of vertices that receive messages from the given vertex
	Outgoing(vertex string) []string

	// RootVertices returns the entry points of the graph
	RootVertices() []string
}

// TopologicalScheduler executes vertices in topological order based on graph structure.
// This is the default scheduler - it provides deterministic, predictable execution
// with minimal overhead.
//
// ALGORITHM:
//   - Sorts vertices lexicographically by name
//   - No priority or resource awareness
//   - O(n log n) where n is frontier size
//
// USE CASES:
//   - Default for most workflows
//   - Debugging (deterministic execution order)
//   - Testing (reproducible results)
type TopologicalScheduler struct{}

// NewTopologicalScheduler creates a new topological scheduler.
func NewTopologicalScheduler() *TopologicalScheduler {
	return &TopologicalScheduler{}
}

// NextBatch returns vertices sorted lexicographically by name.
// This provides deterministic execution order for reproducibility.
func (s *TopologicalScheduler) NextBatch(ctx context.Context, info SchedulerInfo) ([]string, error) {
	if len(info.Frontier) == 0 {
		return []string{}, nil
	}

	batch := make([]string, 0, len(info.Frontier))
	for vertex := range info.Frontier {
		batch = append(batch, vertex)
	}

	// Sort for deterministic order
	sort.Strings(batch)

	return batch, nil
}

// RecordCompletion is a no-op for topological scheduler (stateless).
func (s *TopologicalScheduler) RecordCompletion(ctx context.Context, vertex string, info CompletionInfo) {
	// No-op: TopologicalScheduler doesn't learn from execution history
}

// Ensure TopologicalScheduler implements Scheduler interface
var _ Scheduler = (*TopologicalScheduler)(nil)
