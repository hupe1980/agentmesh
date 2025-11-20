package channel

import (
	"context"
	"sync"
)

// Aggregator combines values using aggregation semantics (sum, max, avg, etc.).
// This mirrors pregel.Aggregator but is defined here to avoid circular dependencies.
type Aggregator interface {
	// Zero returns the identity/initial value for this aggregator
	Zero() any

	// Aggregate combines the current accumulated value with a new contribution
	Aggregate(current, value any) any
}

// AggregateChannel accumulates values using configurable aggregation semantics.
// Unlike TopicChannel (append-only) or LastValueChannel (overwrite), AggregateChannel
// combines all written values using an Aggregator (sum, max, average, etc.).
//
// Use Cases:
//   - Global counters (SumAggregator)
//   - Maximum/minimum tracking (MaxAggregator, MinAggregator)
//   - Statistical computations (AvgAggregator, VarianceAggregator)
//   - Convergence detection (AllTrueAggregator)
//
// Thread-safety: Fully thread-safe for concurrent writes. The mutex serializes
// access to the accumulated value, preventing lost updates even when multiple
// nodes write to the same aggregate key in parallel within a Pregel superstep.
//
// Example:
//
//	// Create channel with sum aggregation
//	sumCh := NewAggregateChannel("total_cost", &SumAggregator{})
//
//	// Each write adds to the accumulated total
//	sumCh.Write(ctx, 10.5) // total: 10.5
//	sumCh.Write(ctx, 5.0)  // total: 15.5
//
//	// Read returns accumulated value
//	total, _ := sumCh.Read(ctx) // 15.5
type AggregateChannel struct {
	name       string
	aggregator Aggregator
	current    any
	version    int64
	mu         sync.RWMutex

	// Copy-on-write cache for Read() operations
	cachedSnapshot any
	cachedVersion  int64
}

// NewAggregateChannel creates a new aggregate channel with the given aggregator.
// The channel is initialized with the aggregator's Zero() value.
func NewAggregateChannel(name string, aggregator Aggregator) *AggregateChannel {
	return &AggregateChannel{
		name:       name,
		aggregator: aggregator,
		current:    aggregator.Zero(),
		version:    0,
	}
}

// Name returns the channel's identifier.
func (ac *AggregateChannel) Name() string {
	return ac.name
}

// Read returns the current accumulated value.
// Uses copy-on-write caching to avoid redundant copies.
func (ac *AggregateChannel) Read(ctx context.Context) (any, error) {
	// Fast path: check if cached snapshot is valid (read lock only)
	ac.mu.RLock()
	if ac.cachedVersion == ac.version && ac.cachedSnapshot != nil {
		snapshot := ac.cachedSnapshot
		ac.mu.RUnlock()
		return snapshot, nil
	}
	ac.mu.RUnlock()

	// Slow path: create new snapshot (write lock to update cache)
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Double-check after acquiring write lock
	if ac.cachedVersion == ac.version && ac.cachedSnapshot != nil {
		return ac.cachedSnapshot, nil
	}

	// Cache current value (shallow copy - aggregators manage deep state)
	ac.cachedSnapshot = ac.current
	ac.cachedVersion = ac.version

	return ac.current, nil
}

// Write combines the given value with the current accumulated value using the aggregator.
// Thread-safe: The mutex ensures serialized access to ac.current, preventing lost updates
// even when multiple nodes write concurrently within the same Pregel superstep.
func (ac *AggregateChannel) Write(ctx context.Context, value any) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Aggregate new value with current (protected by mutex)
	ac.current = ac.aggregator.Aggregate(ac.current, value)
	ac.version++

	// Invalidate cache
	ac.cachedSnapshot = nil

	return nil
}

// Snapshot returns a copy of the current accumulated value.
func (ac *AggregateChannel) Snapshot(ctx context.Context) (any, error) {
	return ac.Read(ctx)
}

// Version returns the current version number for cache invalidation.
func (ac *AggregateChannel) Version() int64 {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.version
}

// Reset clears the accumulated value back to the aggregator's Zero() value.
func (ac *AggregateChannel) Reset(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.current = ac.aggregator.Zero()
	ac.version = 0
	ac.cachedSnapshot = nil
	ac.cachedVersion = 0
	return nil
}

// Clone returns a deep copy of the aggregate channel.
func (ac *AggregateChannel) Clone() VersionedChannel {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return &AggregateChannel{
		name:       ac.name,
		aggregator: ac.aggregator, // Aggregators are stateless, safe to share
		current:    ac.current,    // Shallow copy - aggregator manages state
		version:    ac.version,
		// Don't copy cache - let clone build its own
		cachedSnapshot: nil,
		cachedVersion:  0,
	}
}

// GetAggregator returns the underlying aggregator (for introspection).
func (ac *AggregateChannel) GetAggregator() Aggregator {
	return ac.aggregator
}

// Compile-time interface checks
var (
	_ Channel           = (*AggregateChannel)(nil)
	_ VersionedChannel  = (*AggregateChannel)(nil)
	_ ResettableChannel = (*AggregateChannel)(nil)
)
