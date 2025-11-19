package state

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// VersionedSnapshot represents a point-in-time capture of state with metadata.
// This is different from state.Snapshot which is used for BSP execution.
type VersionedSnapshot struct {
	// ID is a unique identifier for this snapshot (timestamp-based)
	ID string
	// Timestamp is when the snapshot was created
	Timestamp time.Time
	// Data contains the captured state (key-value pairs)
	Data map[string]any
	// Metadata can store additional snapshot information (tags, description, etc.)
	Metadata map[string]string
}

// SnapshotManager provides in-memory versioning and rollback capabilities.
// Snapshots are stored in memory and are lost on process restart.
// For persistent snapshots, use checkpoint.Checkpointer with a durable backend.
type SnapshotManager struct {
	snapshots    map[string]*VersionedSnapshot
	mu           sync.RWMutex
	maxSnapshots int // 0 means unlimited
}

// SnapshotOption configures SnapshotManager behavior.
type SnapshotOption func(*SnapshotManager)

// WithMaxSnapshots sets a limit on the number of retained snapshots.
// When the limit is exceeded, the oldest snapshot is deleted.
// Default is unlimited (0).
func WithMaxSnapshots(maxSnapshots int) SnapshotOption {
	return func(sm *SnapshotManager) {
		sm.maxSnapshots = maxSnapshots
	}
}

// NewSnapshotManager creates a new in-memory snapshot manager.
func NewSnapshotManager(opts ...SnapshotOption) *SnapshotManager {
	sm := &SnapshotManager{
		snapshots: make(map[string]*VersionedSnapshot),
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

// CreateSnapshot captures the current state.
// The snapshot ID is generated from the current timestamp.
// If metadata is provided, it's attached to the snapshot.
func (sm *SnapshotManager) CreateSnapshot(ctx context.Context, data map[string]any, metadata map[string]string) (*VersionedSnapshot, error) {
	if data == nil {
		return nil, fmt.Errorf("snapshot data cannot be nil")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Generate snapshot ID from timestamp
	now := time.Now()
	id := now.Format("20060102150405.000000")

	// Deep copy the data to prevent external mutations
	dataCopy := make(map[string]any, len(data))
	for k, v := range data {
		dataCopy[k] = v
	}

	// Deep copy metadata
	metadataCopy := make(map[string]string, len(metadata))
	for k, v := range metadata {
		metadataCopy[k] = v
	}

	snapshot := &VersionedSnapshot{
		ID:        id,
		Timestamp: now,
		Data:      dataCopy,
		Metadata:  metadataCopy,
	}

	sm.snapshots[id] = snapshot

	// Enforce max snapshots limit if set
	if sm.maxSnapshots > 0 && len(sm.snapshots) > sm.maxSnapshots {
		sm.evictOldestSnapshot()
	}

	return snapshot, nil
}

// RestoreSnapshot loads state from a snapshot.
// Returns a copy of the snapshot data to prevent external mutations.
func (sm *SnapshotManager) RestoreSnapshot(ctx context.Context, snapshotID string) (map[string]any, error) {
	sm.mu.RLock()
	snapshot, exists := sm.snapshots[snapshotID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("snapshot %q not found", snapshotID)
	}

	// Return a copy of the snapshot data
	dataCopy := make(map[string]any, len(snapshot.Data))
	for k, v := range snapshot.Data {
		dataCopy[k] = v
	}

	return dataCopy, nil
}

// GetSnapshot retrieves a snapshot by ID without restoring it.
// Returns a copy of the snapshot to prevent external mutations.
func (sm *SnapshotManager) GetSnapshot(snapshotID string) (*VersionedSnapshot, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshot, exists := sm.snapshots[snapshotID]
	if !exists {
		return nil, fmt.Errorf("snapshot %q not found", snapshotID)
	}

	// Return a copy of the snapshot
	return &VersionedSnapshot{
		ID:        snapshot.ID,
		Timestamp: snapshot.Timestamp,
		Data:      copyMap(snapshot.Data),
		Metadata:  copyStringMap(snapshot.Metadata),
	}, nil
}

// ListSnapshots returns all snapshot IDs sorted by creation time (newest first).
func (sm *SnapshotManager) ListSnapshots() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.snapshots) == 0 {
		return []string{}
	}

	// Create slice of snapshots for sorting
	snapshots := make([]*VersionedSnapshot, 0, len(sm.snapshots))
	for _, s := range sm.snapshots {
		snapshots = append(snapshots, s)
	}

	// Sort by timestamp descending (newest first)
	// Simple bubble sort is fine for small lists
	for i := 0; i < len(snapshots)-1; i++ {
		for j := i + 1; j < len(snapshots); j++ {
			if snapshots[i].Timestamp.Before(snapshots[j].Timestamp) {
				snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
			}
		}
	}

	ids := make([]string, len(snapshots))
	for i, s := range snapshots {
		ids[i] = s.ID
	}
	return ids
}

// DeleteSnapshot removes a snapshot by ID.
func (sm *SnapshotManager) DeleteSnapshot(snapshotID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.snapshots[snapshotID]; !exists {
		return fmt.Errorf("snapshot %q not found", snapshotID)
	}

	delete(sm.snapshots, snapshotID)
	return nil
}

// DeleteAllSnapshots removes all snapshots.
func (sm *SnapshotManager) DeleteAllSnapshots() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.snapshots = make(map[string]*VersionedSnapshot)
}

// Len returns the number of stored snapshots.
func (sm *SnapshotManager) Len() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.snapshots)
}

// evictOldestSnapshot removes the oldest snapshot.
// Caller must hold write lock.
func (sm *SnapshotManager) evictOldestSnapshot() {
	if len(sm.snapshots) == 0 {
		return
	}

	var oldestID string
	var oldestTime time.Time
	first := true

	for id, snapshot := range sm.snapshots {
		if first || snapshot.Timestamp.Before(oldestTime) {
			oldestID = id
			oldestTime = snapshot.Timestamp
			first = false
		}
	}

	delete(sm.snapshots, oldestID)
}

// copyMap creates a shallow copy of a map[string]any.
func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// copyStringMap creates a shallow copy of a map[string]string.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
