package state

import (
	"fmt"
)

// StateStore defines the interface for persisting and retrieving graph state.
// This abstraction enables checkpointing, state snapshots, and distributed execution.
//
//nolint:revive // StateStore is an established API name
type StateStore interface {
	// Save persists the current state snapshot with the given checkpoint ID.
	Save(checkpointID string, state *ChannelState) error

	// Load retrieves a previously saved state snapshot.
	Load(checkpointID string) (*ChannelState, error)

	// Delete removes a checkpoint from storage.
	Delete(checkpointID string) error

	// List returns all available checkpoint IDs.
	List() ([]string, error)
}

// InMemoryStateStore provides a simple in-memory implementation of StateStore.
type InMemoryStateStore struct {
	checkpoints map[string]*ChannelState
}

// NewInMemoryStateStore creates a new in-memory state store.
func NewInMemoryStateStore() *InMemoryStateStore {
	return &InMemoryStateStore{
		checkpoints: make(map[string]*ChannelState),
	}
}

// Save stores a deep copy of the state.
func (s *InMemoryStateStore) Save(checkpointID string, state *ChannelState) error {
	if state == nil {
		return ErrInvalidState
	}

	// Create a deep copy with snapshots of all channels
	snapshot := state.SnapshotAll()
	events := state.MessagesSnapshot()
	msgs := make([]ExecutionResult, len(events))
	for i := range events {
		msgs[i] = *events[i].Clone()
	}

	// Create new state and restore channels
	// Note: We need to recreate channels with the same types as original
	// For now, we create a simple state and populate it
	newState, err := NewChannelState(0) // Unlimited messages
	if err != nil {
		return fmt.Errorf("failed to create state for checkpoint: %w", err)
	}

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
func (s *InMemoryStateStore) Load(checkpointID string) (*ChannelState, error) {
	state, ok := s.checkpoints[checkpointID]
	if !ok {
		return nil, ErrCheckpointNotFound
	}

	// Return a copy to prevent mutations
	snapshot := state.SnapshotAll()
	events := state.MessagesSnapshot()
	msgs := make([]ExecutionResult, len(events))
	for i := range events {
		msgs[i] = *events[i].Clone()
	}

	loaded, err := NewChannelState(0) // Unlimited messages
	if err != nil {
		return nil, fmt.Errorf("failed to create state for loading: %w", err)
	}

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
