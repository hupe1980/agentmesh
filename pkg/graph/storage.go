package graph

import "github.com/hupe1980/agentmesh/pkg/message"

// StateStore defines the interface for persisting and retrieving graph state.
// This abstraction enables checkpointing, state snapshots, and distributed execution.
type StateStore interface {
	// Save persists the current state snapshot with the given checkpoint ID.
	Save(checkpointID string, state *GraphState) error

	// Load retrieves a previously saved state snapshot.
	Load(checkpointID string) (*GraphState, error)

	// Delete removes a checkpoint from storage.
	Delete(checkpointID string) error

	// List returns all available checkpoint IDs.
	List() ([]string, error)
}

// MessageBus defines the interface for message passing between vertices.
// This abstraction enables distributed message delivery and buffering strategies.
type MessageBus interface {
	// Send delivers messages to a target vertex.
	Send(target string, msgs []message.Message) error

	// Receive retrieves all pending messages for a vertex.
	Receive(vertex string) ([]message.Message, error)

	// Clear removes all pending messages for a vertex.
	Clear(vertex string) error

	// Flush ensures all buffered messages are delivered.
	Flush() error
}

// InMemoryStateStore provides a simple in-memory implementation of StateStore.
type InMemoryStateStore struct {
	checkpoints map[string]*GraphState
}

// NewInMemoryStateStore creates a new in-memory state store.
func NewInMemoryStateStore() *InMemoryStateStore {
	return &InMemoryStateStore{
		checkpoints: make(map[string]*GraphState),
	}
}

// Save stores a deep copy of the state.
func (s *InMemoryStateStore) Save(checkpointID string, state *GraphState) error {
	if state == nil {
		return ErrInvalidState
	}

	// Create a deep copy with snapshots of all channels
	snapshot := state.SnapshotAll()
	msgs := cloneMessages(state.MessagesSnapshot())

	// Create new state and restore channels
	// Note: We need to recreate channels with the same types as original
	// For now, we create a simple state and populate it
	newState := NewGraphState(0) // Unlimited messages

	// Copy all channel data except messages (handled separately)
	for key, value := range snapshot {
		if key != "messages" {
			_ = newState.Set(key, value) // Ignore error - internal state reconstruction
		}
	}

	// Add messages
	newState.AddMessages(msgs)

	s.checkpoints[checkpointID] = newState
	return nil
}

// Load retrieves a state snapshot.
func (s *InMemoryStateStore) Load(checkpointID string) (*GraphState, error) {
	state, ok := s.checkpoints[checkpointID]
	if !ok {
		return nil, ErrCheckpointNotFound
	}

	// Return a copy to prevent mutations
	snapshot := state.SnapshotAll()
	msgs := cloneMessages(state.MessagesSnapshot())

	loaded := NewGraphState(0) // Unlimited messages

	// Copy all channel data except messages
	for key, value := range snapshot {
		if key != "messages" {
			_ = loaded.Set(key, value) // Ignore error - internal state reconstruction
		}
	}

	// Add messages
	loaded.AddMessages(msgs)

	return loaded, nil
}

// Delete removes a checkpoint.
func (s *InMemoryStateStore) Delete(checkpointID string) error {
	delete(s.checkpoints, checkpointID)
	return nil
}

// List returns all checkpoint IDs.
func (s *InMemoryStateStore) List() ([]string, error) {
	ids := make([]string, 0, len(s.checkpoints))
	for id := range s.checkpoints {
		ids = append(ids, id)
	}
	return ids, nil
}

// InMemoryMessageBus provides a simple in-memory message buffer.
type InMemoryMessageBus struct {
	mailboxes map[string][]message.Message
}

// NewInMemoryMessageBus creates a new in-memory message bus.
func NewInMemoryMessageBus() *InMemoryMessageBus {
	return &InMemoryMessageBus{
		mailboxes: make(map[string][]message.Message),
	}
}

// Send appends messages to the target vertex's mailbox.
func (b *InMemoryMessageBus) Send(target string, msgs []message.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	b.mailboxes[target] = append(b.mailboxes[target], cloneMessages(msgs)...)
	return nil
}

// Receive returns all messages for a vertex and clears its mailbox.
func (b *InMemoryMessageBus) Receive(vertex string) ([]message.Message, error) {
	msgs := b.mailboxes[vertex]
	if len(msgs) == 0 {
		return nil, nil
	}

	// Return copy and clear mailbox
	result := cloneMessages(msgs)
	delete(b.mailboxes, vertex)
	return result, nil
}

// Clear removes all pending messages for a vertex.
func (b *InMemoryMessageBus) Clear(vertex string) error {
	delete(b.mailboxes, vertex)
	return nil
}

// Flush is a no-op for in-memory implementation.
func (b *InMemoryMessageBus) Flush() error {
	return nil
}
