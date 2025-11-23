package pregel

// Package-level constants for default buffer sizes and shard counts.
// These values are chosen based on performance characteristics:
//   - EventChanBufferSize: Small buffer for event streaming (10 events)
//   - ShardCount: Power of 2 for efficient modulo operation (32 shards)
//   - MaxMailboxSize: Reasonable default that prevents OOM while allowing high throughput
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

	// DefaultMaxMailboxSize is the default mailbox capacity per vertex.
	// This value (10,000 messages) provides a good balance between:
	//   - Memory safety: Prevents unbounded memory growth and OOM crashes
	//   - Performance: Allows high message throughput with minimal blocking
	//   - Backpressure: Naturally throttles producers when consumers are slow
	// Users can override this via RuntimeOptions.MaxMailboxSize.
	DefaultMaxMailboxSize = 10000
)
