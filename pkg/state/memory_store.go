package state

import (
	"context"
	"sync"
)

// MemoryStore implements Store using an in-memory map.
// This is the default store implementation and requires no external dependencies.
//
// Thread-safe: Uses sync.RWMutex for concurrent access.
//
// Use cases:
//   - Default store for simple applications
//   - Testing and development
//   - Single-process execution
//
// For distributed or persistent state, use Redis or DynamoDB stores.
type MemoryStore struct {
	data map[string]any
	mu   sync.RWMutex
}

// NewMemoryStore creates a new in-memory store.
//
// Example:
//
//	store := state.NewMemoryStore()
//	builder := state.NewManagerBuilder(state.WithStore(store))
//	mgr := builder.Build()
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]any),
	}
}

// Get retrieves a value by key.
func (m *MemoryStore) Get(ctx context.Context, key string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.data[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	return value, nil
}

// Set stores a value by key.
func (m *MemoryStore) Set(ctx context.Context, key string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = value
	return nil
}

// Delete removes a value by key.
func (m *MemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[key]; !exists {
		return ErrKeyNotFound
	}

	delete(m.data, key)
	return nil
}

// Keys returns all stored keys.
func (m *MemoryStore) Keys(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}

	return keys, nil
}

// Snapshot returns a point-in-time copy of all data.
func (m *MemoryStore) Snapshot(ctx context.Context) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make(map[string]any, len(m.data))
	for k, v := range m.data {
		snapshot[k] = v
	}

	return snapshot, nil
}

// Restore replaces all data with the given snapshot.
func (m *MemoryStore) Restore(ctx context.Context, snapshot map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Replace data entirely
	m.data = make(map[string]any, len(snapshot))
	for k, v := range snapshot {
		m.data[k] = v
	}

	return nil
}

// Close releases resources (no-op for MemoryStore).
func (m *MemoryStore) Close() error {
	return nil
}

// Len returns the number of keys in the store (useful for testing).
func (m *MemoryStore) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Clear removes all data from the store (useful for testing).
func (m *MemoryStore) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]any)
}

var _ Store = (*MemoryStore)(nil) // Compile-time interface check
