package pregel

import (
	"context"
	"fmt"
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
// Thread-safety: All methods are thread-safe using mutex and channels.
// Backpressure: Send blocks when channel is full until space is available or context cancelled.
// Memory bounds: Each vertex mailbox has a bounded channel (maxSize > 0) or unbounded buffer (maxSize = 0).
// Combiner support: Merges messages for the same target if configured.
type InMemoryMessageBus[M any] struct {
	mu           sync.Mutex
	mailbox      map[string]chan Message[M]
	buffer       map[string][]Message[M] // For unlimited mailboxes (maxSize=0)
	maxSize      int
	combiner     Combiner[M]
	nextFrontier map[string]struct{}
	closed       bool
}

// NewInMemoryMessageBus creates an in-memory message bus with backpressure.
// maxSize controls mailbox capacity per vertex:
//   - maxSize > 0: Uses buffered channels, Send blocks when full (backpressure)
//   - maxSize = 0: Uses unbounded buffer, Send never blocks (legacy behavior)
//
// combiner, if provided, merges messages for the same target.
func NewInMemoryMessageBus[M any](maxSize int, combiner Combiner[M]) *InMemoryMessageBus[M] {
	return &InMemoryMessageBus[M]{
		mailbox:      make(map[string]chan Message[M]),
		buffer:       make(map[string][]Message[M]),
		maxSize:      maxSize,
		combiner:     combiner,
		nextFrontier: make(map[string]struct{}),
	}
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
func (bus *InMemoryMessageBus[M]) sendOne(ctx context.Context, msg Message[M]) error {
	bus.mu.Lock()

	if bus.closed {
		bus.mu.Unlock()
		return fmt.Errorf("message bus is closed")
	}

	// Mark vertex for next frontier
	bus.nextFrontier[msg.To] = struct{}{}

	// Handle unbounded mailboxes (maxSize = 0)
	if bus.maxSize == 0 {
		// Apply combiner if configured
		if bus.combiner != nil {
			if existing := bus.buffer[msg.To]; len(existing) > 0 {
				combined := bus.combiner(existing[0], msg)
				bus.buffer[msg.To] = []Message[M]{combined}
				bus.mu.Unlock()
				return nil
			}
		}

		// Append to unbounded buffer
		bus.buffer[msg.To] = append(bus.buffer[msg.To], msg)
		bus.mu.Unlock()
		return nil
	}

	// Handle bounded mailboxes with backpressure
	ch, exists := bus.mailbox[msg.To]
	if !exists {
		// Create new bounded channel
		ch = make(chan Message[M], bus.maxSize)
		bus.mailbox[msg.To] = ch
	}

	bus.mu.Unlock()

	// Blocking send with context support
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
func (bus *InMemoryMessageBus[M]) Receive(vertex string) ([]Message[M], error) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Handle unbounded mailboxes
	if bus.maxSize == 0 {
		msgs := bus.buffer[vertex]
		if len(msgs) == 0 {
			return nil, nil
		}

		// Remove from buffer
		delete(bus.buffer, vertex)

		// Return a copy to prevent external mutation
		result := make([]Message[M], len(msgs))
		copy(result, msgs)
		return result, nil
	}

	// Handle bounded mailboxes - drain channel WITHOUT removing it
	ch, exists := bus.mailbox[vertex]
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
func (bus *InMemoryMessageBus[M]) Clear(vertex string) error {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Clear unbounded buffer
	delete(bus.buffer, vertex)

	// For bounded channels, just drain them (don't close since sends may be in flight)
	if ch, exists := bus.mailbox[vertex]; exists {
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

	delete(bus.nextFrontier, vertex)
	return nil
}

// Pending returns vertices with pending messages.
func (bus *InMemoryMessageBus[M]) Pending() ([]string, error) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	if len(bus.nextFrontier) == 0 {
		return nil, nil
	}

	vertices := make([]string, 0, len(bus.nextFrontier))
	for name := range bus.nextFrontier {
		vertices = append(vertices, name)
	}

	// Clear frontier after reading
	bus.nextFrontier = make(map[string]struct{})

	return vertices, nil
}

// Close releases resources and closes all channels.
func (bus *InMemoryMessageBus[M]) Close() error {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	if bus.closed {
		return nil
	}

	bus.closed = true

	// Close all bounded mailbox channels
	for _, ch := range bus.mailbox {
		close(ch)
	}

	bus.mailbox = nil
	bus.buffer = nil
	bus.nextFrontier = nil
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
func (bus *InMemoryMessageBus[M]) Stats() MessageBusStats {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	stats := MessageBusStats{}

	// Count unbounded buffer messages
	for _, msgs := range bus.buffer {
		count := len(msgs)
		stats.TotalMessages += count
		stats.VerticesWithMessages++
		if count > stats.LargestMailbox {
			stats.LargestMailbox = count
		}
	}

	// Count bounded channel messages
	for _, ch := range bus.mailbox {
		count := len(ch)
		stats.TotalMessages += count
		if count > 0 {
			stats.VerticesWithMessages++
		}
		if count > stats.LargestMailbox {
			stats.LargestMailbox = count
		}
	}

	return stats
}
