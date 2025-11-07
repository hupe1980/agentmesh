package pregel

import (
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
//   - Maintain backward compatibility with in-memory execution
//
// Implementations:
//   - InMemoryMessageBus: Default, single-process execution
//   - RedisMessageBus: Distributed execution with Redis backend
//   - GRPCMessageBus: Multi-node coordination via gRPC
//   - PersistedMessageBus: Message persistence for replay debugging
type MessageBus[M any] interface {
	// Send delivers messages to target vertices.
	// Returns ErrMailboxFull if any target's mailbox has reached capacity.
	// Implementations must be thread-safe and support concurrent sends.
	Send(messages []Message[M]) error

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

// InMemoryMessageBus is the default implementation using a simple map.
// It provides fast, single-process message delivery with optional size limits.
//
// Thread-safety: All methods are protected by a mutex.
// Memory bounds: Respects maxMailboxSize per vertex (0 = unlimited).
// Combiner support: Merges messages for the same target if configured.
type InMemoryMessageBus[M any] struct {
	mu           sync.Mutex
	mailbox      map[string][]Message[M]
	maxSize      int
	combiner     Combiner[M]
	nextFrontier map[string]struct{}
}

// NewInMemoryMessageBus creates an in-memory message bus.
// maxSize of 0 means unlimited mailbox capacity.
// combiner, if provided, merges messages for the same target.
func NewInMemoryMessageBus[M any](maxSize int, combiner Combiner[M]) *InMemoryMessageBus[M] {
	return &InMemoryMessageBus[M]{
		mailbox:      make(map[string][]Message[M]),
		maxSize:      maxSize,
		combiner:     combiner,
		nextFrontier: make(map[string]struct{}),
	}
}

// Send delivers messages to their target vertices.
// Returns ErrMailboxFull if any message cannot be delivered due to capacity limits.
// The error wraps individual failures for each dropped message.
func (bus *InMemoryMessageBus[M]) Send(messages []Message[M]) error {
	if len(messages) == 0 {
		return nil
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	var errors []error

	for _, msg := range messages {
		if msg.To == "" {
			continue
		}

		// Check mailbox size limit
		if bus.maxSize > 0 {
			currentSize := len(bus.mailbox[msg.To])
			if currentSize >= bus.maxSize {
				err := fmt.Errorf("%w: node %q has %d messages (limit: %d)",
					ErrMailboxFull, msg.To, currentSize, bus.maxSize)
				errors = append(errors, err)
				continue
			}
		}

		// Apply combiner if configured and messages exist
		if bus.combiner != nil {
			if existing, ok := bus.mailbox[msg.To]; ok && len(existing) > 0 {
				combined := bus.combiner(existing[0], msg)
				bus.mailbox[msg.To] = []Message[M]{combined}
				bus.nextFrontier[msg.To] = struct{}{}
				continue
			}
		}

		// Append message to mailbox
		bus.mailbox[msg.To] = append(bus.mailbox[msg.To], msg)
		bus.nextFrontier[msg.To] = struct{}{}
	}

	// Return first error if any occurred
	if len(errors) > 0 {
		return errors[0]
	}
	return nil
}

// Receive retrieves and removes all messages for the given vertex.
func (bus *InMemoryMessageBus[M]) Receive(vertex string) ([]Message[M], error) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	msgs := bus.mailbox[vertex]
	if len(msgs) == 0 {
		return nil, nil
	}

	// Remove from mailbox
	delete(bus.mailbox, vertex)

	// Return a copy to prevent external mutation
	result := make([]Message[M], len(msgs))
	copy(result, msgs)
	return result, nil
}

// Clear removes all messages for the given vertex.
func (bus *InMemoryMessageBus[M]) Clear(vertex string) error {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	delete(bus.mailbox, vertex)
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

// Close releases resources (no-op for in-memory implementation).
func (bus *InMemoryMessageBus[M]) Close() error {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	bus.mailbox = nil
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
	for _, msgs := range bus.mailbox {
		count := len(msgs)
		stats.TotalMessages += count
		stats.VerticesWithMessages++
		if count > stats.LargestMailbox {
			stats.LargestMailbox = count
		}
	}
	return stats
}
