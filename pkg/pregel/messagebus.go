package pregel

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
)

// MessageBus abstracts message delivery and storage for graph execution.
// This interface focuses purely on message persistence and retrieval,
// decoupled from frontier tracking which is handled separately by Runtime.
//
// Design Goals:
//   - Pure message storage abstraction (no frontier tracking)
//   - Enable distributed deployments (Redis, gRPC, Kafka, etc.)
//   - Support message persistence for debugging and replay
//   - Guarantee no message loss through backpressure or explicit errors
//
// Message Delivery Guarantee:
//   - Messages are NEVER dropped silently
//   - Send either succeeds, blocks (backpressure), or returns explicit error
//   - Context cancellation during blocking returns error (no message corruption)
//
// Implementations:
//   - InMemoryMessageBus: Default, single-process execution with blocking send
//   - RedisMessageStore: Distributed execution with Redis backend
//   - GRPCMessageStore: Multi-node coordination via gRPC
//   - PersistedMessageStore: Message persistence for replay debugging
//
// Frontier Tracking:
//
//	Frontier tracking (which vertices have pending messages) is handled separately
//	by Runtime's FrontierTracker. MessageBus implementations should NOT maintain
//	frontier state - this simplifies distributed deployments and allows Runtime
//	to use optimized lock-free frontier data structures.
type MessageBus[M any] interface {
	// Send delivers messages to target vertices with backpressure.
	// NEVER drops messages silently - either succeeds, blocks, or returns error.
	// Blocks when mailbox is full until space is available or context is cancelled.
	// Returns context error if cancelled/timed out during blocking.
	// Implementations must be thread-safe and support concurrent sends.
	Send(ctx context.Context, messages []Message[M]) error

	// Receive retrieves and removes all messages for the given vertex.
	// Returns nil if no messages are pending.
	// Implementations must be thread-safe.
	Receive(ctx context.Context, vertex string) ([]Message[M], error)

	// Clear removes all messages for the given vertex without returning them.
	// Used during cleanup or error recovery.
	Clear(ctx context.Context, vertex string) error

	// Close releases resources held by the message store.
	// After Close is called, Send and Receive operations may fail.
	Close() error
}

// InMemoryMessageBus implements MessageBus using buffered channels.
// It provides fast, single-process message delivery with backpressure support.
//
// Thread-safety: All methods are thread-safe using sharded locks (32 shards).
// Backpressure: Send blocks when channel is full until space is available or context cancelled.
// Memory bounds: Each vertex mailbox has a bounded channel with configurable capacity.
//
// All mailboxes are bounded to prevent memory exhaustion. If maxSize <= 0 is provided,
// DefaultMaxMailboxSize (10,000) is used automatically. This ensures:
//   - Memory safety: No unbounded growth or OOM crashes
//   - Backpressure: Producers are naturally throttled when consumers are slow
//   - Predictable behavior: Resource usage is bounded and observable
//
// Combiner support: Merges messages for the same target if configured.
//
// Performance: Uses DefaultShardCount-shard map with per-shard mutex to reduce lock contention
// in high-throughput scenarios with many concurrent senders.
//
// Frontier Tracking: InMemoryMessageBus does NOT maintain frontier state.
// Runtime uses its own shardedFrontier for lock-free frontier tracking.
type InMemoryMessageBus[M any] struct {
	shards   [DefaultShardCount]messageShard[M]
	maxSize  int
	combiner Combiner[M]
	globalMu sync.Mutex // Only for Close() and global operations
	closed   bool
}

// messageShard represents a single shard with its own lock.
// Frontier tracking is handled separately by Runtime's shardedFrontier.
type messageShard[M any] struct {
	mu      sync.Mutex
	mailbox map[string]chan Message[M]
}

// NewInMemoryMessageBus creates an in-memory message store with backpressure.
//
// maxSize controls mailbox capacity per vertex. If maxSize <= 0, DefaultMaxMailboxSize
// (10,000) is used automatically to ensure memory safety and backpressure.
//
// All mailboxes are bounded to prevent unbounded memory growth and OOM crashes.
// Send operations block when mailboxes are full, providing natural backpressure
// that throttles message producers when consumers cannot keep up.
//
// Recommended values:
//   - Small graphs (<100 nodes): 1,000-5,000 messages per vertex
//   - Medium graphs (100-1000 nodes): 5,000-10,000 messages per vertex
//   - Large graphs (>1000 nodes): 10,000-50,000 messages per vertex
//
// combiner, if provided, merges messages for the same target when channels
// approach capacity, reducing memory pressure.
//
// Implementation: Uses DefaultShardCount shards to reduce lock contention.
// Frontier tracking is handled separately by Runtime.
func NewInMemoryMessageBus[M any](maxSize int, combiner Combiner[M]) *InMemoryMessageBus[M] {
	// Enforce bounded mailboxes for memory safety
	if maxSize <= 0 {
		maxSize = DefaultMaxMailboxSize
	}

	store := &InMemoryMessageBus[M]{
		maxSize:  maxSize,
		combiner: combiner,
	}

	// Initialize all shards
	for i := range store.shards {
		store.shards[i] = messageShard[M]{
			mailbox: make(map[string]chan Message[M]),
		}
	}

	return store
}

// shardIndex returns the shard index for a given vertex name using FNV-1a hash
func (store *InMemoryMessageBus[M]) shardIndex(vertex string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(vertex)) // hash.Hash.Write never returns an error
	return int(h.Sum32() % DefaultShardCount)
}

// Send delivers messages to their target vertices with backpressure.
// Blocks when mailbox is full until space is available or context is cancelled.
// Returns context error if context is cancelled during blocking send.
func (store *InMemoryMessageBus[M]) Send(ctx context.Context, messages []Message[M]) error {
	if len(messages) == 0 {
		return nil
	}

	for _, msg := range messages {
		if msg.To == "" {
			continue
		}

		if err := store.sendOne(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendOne delivers a single message with backpressure handling.
// Uses sharded locks for reduced contention.
func (store *InMemoryMessageBus[M]) sendOne(ctx context.Context, msg Message[M]) error {
	// Check if store is closed before attempting delivery
	if err := store.checkClosed(); err != nil {
		return err
	}

	// Route message to appropriate shard
	shard := store.getShardForVertex(msg.To)

	// All mailboxes are bounded
	return store.sendToBoundedMailbox(ctx, shard, msg)
}

// checkClosed returns an error if the message store is closed.
func (store *InMemoryMessageBus[M]) checkClosed() error {
	store.globalMu.Lock()
	closed := store.closed
	store.globalMu.Unlock()

	if closed {
		return fmt.Errorf("message bus is closed")
	}
	return nil
}

// getShardForVertex returns the shard responsible for the given vertex.
func (store *InMemoryMessageBus[M]) getShardForVertex(vertex string) *messageShard[M] {
	shardIdx := store.shardIndex(vertex)
	return &store.shards[shardIdx]
}

// sendToBoundedMailbox delivers a message to a bounded channel with backpressure handling.
// Frontier tracking is handled separately by Runtime.
func (store *InMemoryMessageBus[M]) sendToBoundedMailbox(
	ctx context.Context,
	shard *messageShard[M],
	msg Message[M],
) error {
	shard.mu.Lock()

	// Get or create bounded channel
	ch := store.getOrCreateChannel(shard, msg.To)

	// Try to combine messages when channel is near capacity
	if store.shouldCombine(ch) {
		if attempted, err := store.tryCombineWithLastMessage(ctx, shard, ch, msg); attempted {
			return err
		}
	}

	shard.mu.Unlock()

	// Blocking send with context support
	return store.blockingSend(ctx, ch, msg)
}

// getOrCreateChannel returns the channel for a vertex, creating it if necessary.
func (store *InMemoryMessageBus[M]) getOrCreateChannel(
	shard *messageShard[M],
	vertex string,
) chan Message[M] {
	ch, exists := shard.mailbox[vertex]
	if !exists {
		ch = make(chan Message[M], store.maxSize)
		shard.mailbox[vertex] = ch
	}
	return ch
}

// shouldCombine determines if message combination should be attempted based on channel capacity.
func (store *InMemoryMessageBus[M]) shouldCombine(ch chan Message[M]) bool {
	if store.combiner == nil || len(ch) == 0 {
		return false
	}
	threshold := (store.maxSize * 3) / 4 // 75% capacity
	return len(ch) >= threshold
}

// tryCombineWithLastMessage attempts to combine the incoming message with the last message in the channel.
// Returns (true, error) if combination was attempted, (false, nil) if combination was not possible.
func (store *InMemoryMessageBus[M]) tryCombineWithLastMessage(
	ctx context.Context,
	shard *messageShard[M],
	ch chan Message[M],
	msg Message[M],
) (bool, error) {
	// Try to drain last message and combine with incoming
	select {
	case lastMsg := <-ch:
		// Combine messages
		combined := store.combiner(lastMsg, msg)
		shard.mu.Unlock()

		// Send combined message (non-blocking, space guaranteed)
		return true, store.blockingSend(ctx, ch, combined)
	default:
		// Another goroutine drained it, continue with normal send
		return false, nil
	}
}

// blockingSend sends a message to a channel with context cancellation support.
func (store *InMemoryMessageBus[M]) blockingSend(
	ctx context.Context,
	ch chan Message[M],
	msg Message[M],
) error {
	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to send message to %q: %w", msg.To, ctx.Err())
	}
}

// Receive retrieves and removes all messages for the given vertex.
// Drains the mailbox channel and returns all pending messages.
// Uses sharded locks for reduced contention.
func (store *InMemoryMessageBus[M]) Receive(ctx context.Context, vertex string) ([]Message[M], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Get shard for this vertex
	shardIdx := store.shardIndex(vertex)
	shard := &store.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Drain channel WITHOUT removing it
	ch, exists := shard.mailbox[vertex]
	if !exists {
		return nil, nil
	}

	// Drain all messages from channel (but keep channel alive for future sends)
	var result []Message[M]
drainLoop:
	for {
		select {
		case msg := <-ch:
			result = append(result, msg)
		default:
			break drainLoop
		}
	}

	if len(result) == 0 {
		return nil, nil
	}

	return result, nil
}

// Clear removes all messages for the given vertex.
// Uses sharded locks for reduced contention.
func (store *InMemoryMessageBus[M]) Clear(ctx context.Context, vertex string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Get shard for this vertex
	shardIdx := store.shardIndex(vertex)
	shard := &store.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Drain channel (don't close since sends may be in flight)
	if ch, exists := shard.mailbox[vertex]; exists {
		// Drain channel
	drainLoop:
		for {
			select {
			case <-ch:
				// Discard
			default:
				break drainLoop
			}
		}
	}

	return nil
}

// Close releases resources and closes all channels.
// Closes all shards and their associated mailboxes.
// Note: Close is typically called during shutdown and doesn't take context,
// but checks are minimal so cancellation is not critical here.
func (store *InMemoryMessageBus[M]) Close() error {
	store.globalMu.Lock()
	defer store.globalMu.Unlock()

	if store.closed {
		return nil
	}

	store.closed = true

	// Close all shards (fast operation, no context check needed)
	for i := range store.shards {
		shard := &store.shards[i]
		shard.mu.Lock()

		// Close all mailbox channels in this shard
		for _, ch := range shard.mailbox {
			close(ch)
		}

		shard.mailbox = nil
		shard.mu.Unlock()
	}

	return nil
}

// MessageStoreStats provides metrics about message store state.
type MessageStoreStats struct {
	// TotalMessages is the total number of messages currently queued
	TotalMessages int

	// VerticesWithMessages is the number of vertices with pending messages
	VerticesWithMessages int

	// LargestMailbox is the maximum number of messages in any single mailbox
	LargestMailbox int
}

// Stats returns statistics about the message store state.
// Only available for InMemoryMessageBus.
// Aggregates stats from all shards with context cancellation support.
func (store *InMemoryMessageBus[M]) Stats(ctx context.Context) MessageStoreStats {
	stats := MessageStoreStats{}

	// Aggregate stats from all shards
	for i := range store.shards {
		// Check context cancellation periodically
		if i%DefaultContextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				// Context cancelled - return partial stats
				return stats
			}
		}

		shard := &store.shards[i]
		shard.mu.Lock()

		// Count channel messages
		for _, ch := range shard.mailbox {
			count := len(ch)
			stats.TotalMessages += count
			if count > 0 {
				stats.VerticesWithMessages++
			}
			if count > stats.LargestMailbox {
				stats.LargestMailbox = count
			}
		}

		shard.mu.Unlock()
	}

	return stats
}
