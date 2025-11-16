package pregel

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
)

// MessageBus abstracts message delivery and storage, enabling pluggable backends
// for distributed execution. Implementations must be thread-safe.
//
// Design Goals:
//   - Decouple mailbox storage from Runtime execution logic
//   - Enable distributed deployments (Redis, gRPC, Kafka, etc.)
//   - Support message persistence for debugging and replay
//   - Support backpressure to prevent message loss
//
// Implementations:
//   - InMemoryMessageBus: Default, single-process execution with blocking send
//   - RedisMessageBus: Distributed execution with Redis backend
//   - GRPCMessageBus: Multi-node coordination via gRPC
//   - PersistedMessageBus: Message persistence for replay debugging
type MessageBus[M any] interface {
	// Send delivers messages to target vertices with backpressure.
	// Blocks when mailbox is full until space is available or context is cancelled.
	// Returns context error if cancelled/timed out during blocking.
	// Implementations must be thread-safe and support concurrent sends.
	Send(ctx context.Context, messages []Message[M]) error

	// Receive retrieves and removes all messages for the given vertex.
	// Returns nil if no messages are pending.
	// Implementations must be thread-safe.
	Receive(vertex string) ([]Message[M], error)

	// Clear removes all messages for the given vertex without returning them.
	// Used during cleanup or error recovery.
	Clear(vertex string) error

	// Pending returns the vertices that have messages waiting.
	// Used to build the execution frontier.
	Pending() ([]string, error)

	// Close releases resources held by the message bus.
	// After Close is called, Send and Receive operations may fail.
	Close() error
}

// InMemoryMessageBus is the default implementation using buffered channels.
// It provides fast, single-process message delivery with backpressure support.
//
// Thread-safety: All methods are thread-safe using sharded locks (32 shards).
// Backpressure: Send blocks when channel is full until space is available or context cancelled.
// Memory bounds: Each vertex mailbox has a bounded channel (maxSize > 0) or unbounded buffer (maxSize = 0).
// Combiner support: Merges messages for the same target if configured.
//
// Performance: Uses DefaultShardCount-shard map with per-shard mutex to reduce lock contention
// in high-throughput scenarios with many concurrent senders.
type InMemoryMessageBus[M any] struct {
	shards   [DefaultShardCount]messageShard[M]
	maxSize  int
	combiner Combiner[M]
	globalMu sync.Mutex // Only for Close() and global operations
	closed   bool
}

// messageShard represents a single shard with its own lock
type messageShard[M any] struct {
	mu           sync.Mutex
	mailbox      map[string]chan Message[M]
	buffer       map[string][]Message[M]
	nextFrontier map[string]struct{}
}

// NewInMemoryMessageBus creates an in-memory message bus with backpressure.
// maxSize controls mailbox capacity per vertex:
//   - maxSize > 0: Uses buffered channels, Send blocks when full (backpressure)
//   - maxSize = 0: Uses unbounded buffer, Send never blocks (legacy behavior)
//
// combiner, if provided, merges messages for the same target.
//
// Implementation: Uses DefaultShardCount shards to reduce lock contention.
func NewInMemoryMessageBus[M any](maxSize int, combiner Combiner[M]) *InMemoryMessageBus[M] {
	bus := &InMemoryMessageBus[M]{
		maxSize:  maxSize,
		combiner: combiner,
	}

	// Initialize all shards
	for i := range bus.shards {
		bus.shards[i] = messageShard[M]{
			mailbox:      make(map[string]chan Message[M]),
			buffer:       make(map[string][]Message[M]),
			nextFrontier: make(map[string]struct{}),
		}
	}

	return bus
}

// shardIndex returns the shard index for a given vertex name using FNV-1a hash
func (bus *InMemoryMessageBus[M]) shardIndex(vertex string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(vertex)) // hash.Hash.Write never returns an error
	return int(h.Sum32() % DefaultShardCount)
}

// Send delivers messages to their target vertices with backpressure.
// For bounded mailboxes (maxSize > 0), blocks when full until space is available.
// For unbounded mailboxes (maxSize = 0), never blocks.
// Returns context error if context is cancelled during blocking send.
func (bus *InMemoryMessageBus[M]) Send(ctx context.Context, messages []Message[M]) error {
	if len(messages) == 0 {
		return nil
	}

	for _, msg := range messages {
		if msg.To == "" {
			continue
		}

		if err := bus.sendOne(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendOne delivers a single message with backpressure handling.
// Uses sharded locks for reduced contention.
func (bus *InMemoryMessageBus[M]) sendOne(ctx context.Context, msg Message[M]) error {
	// Check if bus is closed before attempting delivery
	if err := bus.checkClosed(); err != nil {
		return err
	}

	// Route message to appropriate shard
	shard := bus.getShardForVertex(msg.To)

	// Deliver message using either unbounded or bounded strategy
	if bus.maxSize == 0 {
		return bus.sendToUnboundedMailbox(shard, msg)
	}
	return bus.sendToBoundedMailbox(ctx, shard, msg)
}

// checkClosed returns an error if the message bus is closed.
func (bus *InMemoryMessageBus[M]) checkClosed() error {
	bus.globalMu.Lock()
	closed := bus.closed
	bus.globalMu.Unlock()

	if closed {
		return fmt.Errorf("message bus is closed")
	}
	return nil
}

// getShardForVertex returns the shard responsible for the given vertex.
func (bus *InMemoryMessageBus[M]) getShardForVertex(vertex string) *messageShard[M] {
	shardIdx := bus.shardIndex(vertex)
	return &bus.shards[shardIdx]
}

// sendToUnboundedMailbox delivers a message to an unbounded buffer, applying combiner if configured.
func (bus *InMemoryMessageBus[M]) sendToUnboundedMailbox(shard *messageShard[M], msg Message[M]) error {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Mark vertex for next frontier
	shard.nextFrontier[msg.To] = struct{}{}

	// Try to combine with existing message if combiner is configured
	if bus.combiner != nil {
		if existing := shard.buffer[msg.To]; len(existing) > 0 {
			combined := bus.combiner(existing[0], msg)
			shard.buffer[msg.To] = []Message[M]{combined}
			return nil
		}
	}

	// Append to unbounded buffer
	shard.buffer[msg.To] = append(shard.buffer[msg.To], msg)
	return nil
}

// sendToBoundedMailbox delivers a message to a bounded channel with backpressure handling.
func (bus *InMemoryMessageBus[M]) sendToBoundedMailbox(
	ctx context.Context,
	shard *messageShard[M],
	msg Message[M],
) error {
	shard.mu.Lock()

	// Mark vertex for next frontier
	shard.nextFrontier[msg.To] = struct{}{}

	// Get or create bounded channel
	ch := bus.getOrCreateChannel(shard, msg.To)

	// Try to combine messages when channel is near capacity
	if bus.shouldCombine(ch) {
		if attempted, err := bus.tryCombineWithLastMessage(ctx, shard, ch, msg); attempted {
			return err
		}
	}

	shard.mu.Unlock()

	// Blocking send with context support
	return bus.blockingSend(ctx, ch, msg)
}

// getOrCreateChannel returns the channel for a vertex, creating it if necessary.
func (bus *InMemoryMessageBus[M]) getOrCreateChannel(
	shard *messageShard[M],
	vertex string,
) chan Message[M] {
	ch, exists := shard.mailbox[vertex]
	if !exists {
		ch = make(chan Message[M], bus.maxSize)
		shard.mailbox[vertex] = ch
	}
	return ch
}

// shouldCombine determines if message combination should be attempted based on channel capacity.
func (bus *InMemoryMessageBus[M]) shouldCombine(ch chan Message[M]) bool {
	if bus.combiner == nil || len(ch) == 0 {
		return false
	}
	threshold := (bus.maxSize * 3) / 4 // 75% capacity
	return len(ch) >= threshold
}

// tryCombineWithLastMessage attempts to combine the incoming message with the last message in the channel.
// Returns (true, error) if combination was attempted, (false, nil) if combination was not possible.
func (bus *InMemoryMessageBus[M]) tryCombineWithLastMessage(
	ctx context.Context,
	shard *messageShard[M],
	ch chan Message[M],
	msg Message[M],
) (bool, error) {
	// Try to drain last message and combine with incoming
	select {
	case lastMsg := <-ch:
		// Combine messages
		combined := bus.combiner(lastMsg, msg)
		shard.mu.Unlock()

		// Send combined message (non-blocking, space guaranteed)
		return true, bus.blockingSend(ctx, ch, combined)
	default:
		// Another goroutine drained it, continue with normal send
		return false, nil
	}
}

// blockingSend sends a message to a channel with context cancellation support.
func (bus *InMemoryMessageBus[M]) blockingSend(
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
// For unbounded mailboxes, returns buffered messages.
// For bounded mailboxes, drains the channel.
// Uses sharded locks for reduced contention.
func (bus *InMemoryMessageBus[M]) Receive(vertex string) ([]Message[M], error) {
	// Get shard for this vertex
	shardIdx := bus.shardIndex(vertex)
	shard := &bus.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Handle unbounded mailboxes
	if bus.maxSize == 0 {
		msgs := shard.buffer[vertex]
		if len(msgs) == 0 {
			return nil, nil
		}

		// Remove from buffer
		delete(shard.buffer, vertex)

		// Return a copy to prevent external mutation
		result := make([]Message[M], len(msgs))
		copy(result, msgs)
		return result, nil
	}

	// Handle bounded mailboxes - drain channel WITHOUT removing it
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
func (bus *InMemoryMessageBus[M]) Clear(vertex string) error {
	// Get shard for this vertex
	shardIdx := bus.shardIndex(vertex)
	shard := &bus.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Clear unbounded buffer
	delete(shard.buffer, vertex)

	// For bounded channels, just drain them (don't close since sends may be in flight)
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

	delete(shard.nextFrontier, vertex)
	return nil
}

// Pending returns vertices with pending messages.
// Iterates all shards to collect frontier vertices.
func (bus *InMemoryMessageBus[M]) Pending() ([]string, error) {
	var vertices []string

	// Collect from all shards
	for i := range bus.shards {
		shard := &bus.shards[i]
		shard.mu.Lock()

		for name := range shard.nextFrontier {
			vertices = append(vertices, name)
		}

		// Clear frontier after reading
		shard.nextFrontier = make(map[string]struct{})
		shard.mu.Unlock()
	}

	if len(vertices) == 0 {
		return nil, nil
	}

	return vertices, nil
}

// Close releases resources and closes all channels.
// Closes all shards and their associated mailboxes.
func (bus *InMemoryMessageBus[M]) Close() error {
	bus.globalMu.Lock()
	defer bus.globalMu.Unlock()

	if bus.closed {
		return nil
	}

	bus.closed = true

	// Close all shards
	for i := range bus.shards {
		shard := &bus.shards[i]
		shard.mu.Lock()

		// Close all bounded mailbox channels in this shard
		for _, ch := range shard.mailbox {
			close(ch)
		}

		shard.mailbox = nil
		shard.buffer = nil
		shard.nextFrontier = nil
		shard.mu.Unlock()
	}

	return nil
}

// MessageBusStats provides metrics about message bus state.
type MessageBusStats struct {
	// TotalMessages is the total number of messages currently queued
	TotalMessages int

	// VerticesWithMessages is the number of vertices with pending messages
	VerticesWithMessages int

	// LargestMailbox is the maximum number of messages in any single mailbox
	LargestMailbox int
}

// Stats returns statistics about the message bus state.
// Only available for InMemoryMessageBus.
// Aggregates stats from all shards.
func (bus *InMemoryMessageBus[M]) Stats() MessageBusStats {
	stats := MessageBusStats{}

	// Aggregate stats from all shards
	for i := range bus.shards {
		shard := &bus.shards[i]
		shard.mu.Lock()

		// Count unbounded buffer messages
		for _, msgs := range shard.buffer {
			count := len(msgs)
			stats.TotalMessages += count
			stats.VerticesWithMessages++
			if count > stats.LargestMailbox {
				stats.LargestMailbox = count
			}
		}

		// Count bounded channel messages
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
