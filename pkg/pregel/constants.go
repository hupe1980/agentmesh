package pregel

// Package-level constants for default buffer sizes and shard counts.
// These values are chosen based on performance characteristics:
//   - EventChanBufferSize: Small buffer for event streaming (10 events)
//   - ShardCount: Power of 2 for efficient modulo operation (32 shards)
//
// Users can override these via configuration options where applicable.
const (
	// DefaultEventChanBufferSize is the default buffer size for event channels
	// in the runtime. A small buffer (10) provides good responsiveness while
	// preventing excessive memory usage.
	DefaultEventChanBufferSize = 10

	// DefaultShardCount is the number of shards used in InMemoryMessageBus
	// to reduce lock contention. 32 provides good parallelism for most workloads
	// while keeping memory overhead low.
	DefaultShardCount = 32
)
