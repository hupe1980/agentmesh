package state

import "context"

// Store is the pluggable storage backend interface for the unified state manager.
// Implementations can use in-memory maps, Redis, DynamoDB, PostgreSQL, or any other storage system.
//
// Design Philosophy:
// - Store is the lowest-level storage abstraction
// - Channels are built on top of Store for semantic data flow
// - Manager coordinates Store, Channels, and Type safety
//
// Thread-safety: Implementations MUST be safe for concurrent use.
type Store interface {
	// Get retrieves a value by key.
	// Returns ErrKeyNotFound if the key doesn't exist.
	Get(ctx context.Context, key string) (any, error)

	// Set stores a value by key.
	// If the key exists, it will be overwritten.
	Set(ctx context.Context, key string, value any) error

	// Delete removes a value by key.
	// Returns nil if the key doesn't exist (idempotent).
	Delete(ctx context.Context, key string) error

	// Keys returns all stored keys.
	// The returned slice may be empty if no keys exist.
	Keys(ctx context.Context) ([]string, error)

	// Snapshot returns a point-in-time copy of all data.
	// Used for checkpointing and state versioning.
	// The returned map is a copy; mutations won't affect the store.
	Snapshot(ctx context.Context) (map[string]any, error)

	// Restore replaces all data with the given snapshot.
	// Used for checkpoint restoration and time-travel.
	// This is a destructive operation - existing data will be replaced.
	Restore(ctx context.Context, snapshot map[string]any) error

	// Close releases any resources held by the store.
	// After Close is called, the store should not be used.
	Close() error
}

// ErrKeyNotFound is returned when a key doesn't exist in the store.
var ErrKeyNotFound = &StoreError{msg: "key not found"}

// StoreError represents an error from the store layer.
type StoreError struct {
	msg string
}

func (e *StoreError) Error() string {
	return e.msg
}
