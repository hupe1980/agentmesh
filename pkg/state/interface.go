package state

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/channel"
)

// Manager is the interface for state management operations.
// This abstraction allows for different implementations (in-memory, distributed, etc.)
// and improves testability by enabling mock implementations.
//
// Core responsibilities:
//   - State access: GetChannel for direct channel manipulation
//   - State updates: Apply batch updates to multiple keys
//   - Snapshots: Create point-in-time captures and restore from them
//   - Read views: Create BSP-safe concurrent read snapshots
//   - Checkpoints: Load persistent state from checkpoint storage
//   - Lifecycle: Close and release resources
type Manager interface {
	// GetChannel retrieves the underlying channel for a key.
	// Used for direct channel manipulation and advanced operations.
	GetChannel(name string) channel.Channel

	// ApplyUpdates applies a map of updates to the manager.
	// For registered list keys, values are appended. For regular keys, values are set/replaced.
	ApplyUpdates(ctx context.Context, updates map[string]any) error

	// Snapshot creates a point-in-time capture of all state.
	// Includes both channel values and optional metadata.
	Snapshot(ctx context.Context, metadata map[string]string) (*VersionedSnapshot, error)

	// Restore loads state from a snapshot by ID.
	Restore(ctx context.Context, snapshotID string) error

	// CreateReadView creates a read-only view of the current state.
	// Used for BSP execution to allow concurrent reads without mutations.
	CreateReadView(ctx context.Context) (*ReadView, error)

	// LoadCheckpoint loads state from persistent checkpoint storage.
	// Only available if checkpointer was configured.
	LoadCheckpoint(ctx context.Context) error

	// ListSnapshots returns all in-memory snapshot IDs (newest first).
	ListSnapshots() []string

	// DeleteSnapshot removes an in-memory snapshot.
	DeleteSnapshot(snapshotID string) error

	// RegisteredKeys returns all registered key names.
	RegisteredKeys() []string

	// Close closes the manager and releases resources.
	Close() error
}

// Ensure manager implements Manager interface.
var _ Manager = (*manager)(nil)
