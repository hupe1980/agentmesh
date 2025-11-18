package state

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Manager is the unified facade for state management.
// It coordinates:
// - Store: Pluggable storage backend (memory, Redis, DynamoDB, etc.)
// - ChannelRegistry: Channel-based storage with semantic behaviors
// - TypeRegistry: Runtime type validation for Key[T] types
// - SnapshotManager: In-memory versioning for rollback
// - Checkpointer: Persistent checkpointing (optional)
//
// Architecture: Channels ARE the storage layer, Keys are type-safe accessors.
type Manager struct {
	store           Store
	channels        *ChannelRegistry
	types           *TypeRegistry
	snapshots       *SnapshotManager
	checkpointer    checkpoint.Checkpointer
	checkpointRunID string
}

// ManagerOption configures Manager behavior.
type ManagerOption func(*Manager)

// WithStore sets the storage backend (default: MemoryStore).
func WithStore(store Store) ManagerOption {
	return func(m *Manager) {
		m.store = store
	}
}

// WithCheckpointer enables persistent checkpointing.
func WithCheckpointer(cp checkpoint.Checkpointer, runID string) ManagerOption {
	return func(m *Manager) {
		m.checkpointer = cp
		m.checkpointRunID = runID
	}
}

// WithMaxSnapshots limits in-memory snapshot retention.
func WithMaxSnapshotsLimit(max int) ManagerOption {
	return func(m *Manager) {
		m.snapshots = NewSnapshotManager(WithMaxSnapshots(max))
	}
}

// NewManager creates a new unified state manager.
// Default configuration:
// - MemoryStore for storage
// - No checkpointing
// - Unlimited in-memory snapshots
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		store:     NewMemoryStore(),
		channels:  NewChannelRegistry(),
		types:     NewTypeRegistry(),
		snapshots: NewSnapshotManager(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// RegisterKey registers a Key[T] with the manager.
// This creates the corresponding LastValueChannel and registers the type.
// If the key is already registered, this is a no-op (idempotent).
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	manager.RegisterKey(counterKey)
func RegisterKey[T any](m *Manager, key Key[T]) error {
	valueType := reflect.TypeOf(key.zero)

	// Check if already registered - validate type even if channel exists
	if m.channels.GetChannel(key.name) != nil {
		// Validate that the existing registration matches this type
		return m.types.RegisterKey(key.name, valueType, false)
	}

	return m.registerKeyWithValue(key.name, valueType, false, 0, key.zero)
}

// RegisterListKey registers a ListKey[T] with the manager.
// This creates the corresponding TopicChannel with maxSize limit and registers the type.
// If the key is already registered, this is a no-op (idempotent).
//
// Example:
//
//	messagesKey := NewListKey[string]("messages", 100)
//	manager.RegisterListKey(messagesKey)
func RegisterListKey[T any](m *Manager, key ListKey[T]) error {
	// For list keys, the element type is T, not []T
	var zero T
	valueType := reflect.TypeOf(zero)

	// Check if already registered - validate type even if channel exists
	if m.channels.GetChannel(key.name) != nil {
		// Validate that the existing registration matches this type
		return m.types.RegisterKey(key.name, valueType, true)
	}

	return m.registerKey(key.name, valueType, true, key.maxSize)
}

// registerKey is the internal registration logic (without initial value).
func (m *Manager) registerKey(name string, valueType reflect.Type, isList bool, maxSize int) error {
	return m.registerKeyWithValue(name, valueType, isList, maxSize, nil)
}

// registerKeyWithValue is the internal registration logic with initial value.
func (m *Manager) registerKeyWithValue(name string, valueType reflect.Type, isList bool, maxSize int, initialValue any) error {
	// Register type in TypeRegistry
	if err := m.types.RegisterKey(name, valueType, isList); err != nil {
		return fmt.Errorf("type registration failed: %w", err)
	}

	// Create appropriate channel
	var ch channel.Channel
	if isList {
		// TopicChannel for list keys (append semantics)
		ch = channel.NewTopicChannel(name, maxSize)
		if err := m.channels.RegisterChannel(name, ch, TopicBehavior); err != nil {
			return fmt.Errorf("channel registration failed: %w", err)
		}
		// Don't initialize list channels - they start empty
	} else {
		// LastValueChannel for regular keys (replace semantics)
		ch = channel.NewLastValueChannel(name)
		if err := m.channels.RegisterChannel(name, ch, LastValueBehavior); err != nil {
			return fmt.Errorf("channel registration failed: %w", err)
		}
		// Initialize with provided value or type's zero value to prevent nil panics
		var valueToWrite any
		if initialValue != nil {
			valueToWrite = initialValue
		} else if valueType != nil {
			valueToWrite = reflect.Zero(valueType).Interface()
		}
		// Write initial value to channel
		ctx := context.Background()
		if err := ch.Write(ctx, valueToWrite); err != nil {
			return fmt.Errorf("failed to initialize channel with value: %w", err)
		}
	}

	return nil
}

// GetFromManager retrieves a typed value from state.
// Type safety is enforced by the Key[T] generic parameter.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	value := GetFromManager(ctx, manager, counterKey)
func GetFromManager[T any](ctx context.Context, m *Manager, key Key[T]) (T, error) {
	var zero T

	// Read from channel
	value, err := m.channels.GetChannelValue(ctx, key.name)
	if err != nil {
		return zero, err
	}

	// Handle nil/empty channel
	if value == nil {
		return key.zero, nil
	}

	// Type assertion
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("type mismatch for key %q: expected %T, got %T", key.name, zero, value)
	}

	return typed, nil
}

// SetInManager updates a typed value in state.
// Type safety is enforced at runtime through TypeRegistry validation.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	err := SetInManager(ctx, manager, counterKey, 42)
func SetInManager[T any](ctx context.Context, m *Manager, key Key[T], value T) error {
	// Validate type
	if err := m.types.ValidateType(key.name, value); err != nil {
		return fmt.Errorf("type validation failed: %w", err)
	}

	// Write to channel
	if err := m.channels.WriteValue(ctx, key.name, value); err != nil {
		return fmt.Errorf("channel write failed: %w", err)
	}

	// Write to store for persistence
	if err := m.store.Set(ctx, key.name, value); err != nil {
		return fmt.Errorf("store write failed: %w", err)
	}

	return nil
}

// AppendToManager adds a value to a list.
// Type safety is enforced for the element type T.
//
// Example:
//
//	messagesKey := NewListKey[string]("messages", 100)
//	err := AppendToManager(ctx, manager, messagesKey, "Hello")
func AppendToManager[T any](ctx context.Context, m *Manager, key ListKey[T], value T) error {
	// Validate type (element type, not slice type)
	if err := m.types.ValidateType(key.name, []T{value}); err != nil {
		return fmt.Errorf("type validation failed: %w", err)
	}

	// Write to channel (TopicChannel handles appending)
	if err := m.channels.WriteValue(ctx, key.name, value); err != nil {
		return fmt.Errorf("channel write failed: %w", err)
	}

	// For persistence, we need to get current list, append, then store
	currentList, err := m.channels.GetChannelValue(ctx, key.name)
	if err != nil {
		return fmt.Errorf("failed to get current list: %w", err)
	}

	var updatedList []T
	if currentList != nil {
		if existing, ok := currentList.([]T); ok {
			updatedList = append(existing, value)
		} else {
			// Current value is not a list, start fresh
			updatedList = []T{value}
		}
	} else {
		updatedList = []T{value}
	}

	if err := m.store.Set(ctx, key.name, updatedList); err != nil {
		return fmt.Errorf("store write failed: %w", err)
	}

	return nil
}

// GetChannel retrieves the underlying channel for a key.
// Useful for advanced operations like direct channel manipulation.
//
// Example:
//
//	ch := manager.GetChannel("messages")
//	value, err := ch.Read(ctx)
func (m *Manager) GetChannel(name string) channel.Channel {
	return m.channels.GetChannel(name)
}

// Snapshot creates a point-in-time capture of all state.
// The snapshot includes both channel values and metadata.
func (m *Manager) Snapshot(ctx context.Context, metadata map[string]string) (*VersionedSnapshot, error) {
	// Capture channel state
	data, err := m.channels.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("channel snapshot failed: %w", err)
	}

	// Create snapshot
	snapshot, err := m.snapshots.CreateSnapshot(ctx, data, metadata)
	if err != nil {
		return nil, fmt.Errorf("snapshot creation failed: %w", err)
	}

	// If checkpointer is configured, persist to durable storage
	if m.checkpointer != nil && m.checkpointRunID != "" {
		cp := &checkpoint.Checkpoint{
			RunID:    m.checkpointRunID,
			State:    data,
			Metadata: convertMetadata(metadata),
		}
		if err := m.checkpointer.Save(ctx, cp); err != nil {
			// Log error but don't fail the snapshot
			// In-memory snapshot is still valid
			fmt.Printf("warning: checkpoint save failed: %v\n", err)
		}
	}

	return snapshot, nil
}

// Restore loads state from a snapshot.
func (m *Manager) Restore(ctx context.Context, snapshotID string) error {
	// Load snapshot data
	data, err := m.snapshots.RestoreSnapshot(ctx, snapshotID)
	if err != nil {
		return fmt.Errorf("snapshot restore failed: %w", err)
	}

	// Restore to channels
	if err := m.channels.Restore(ctx, data); err != nil {
		return fmt.Errorf("channel restore failed: %w", err)
	}

	// Restore to store
	if err := m.store.Restore(ctx, data); err != nil {
		return fmt.Errorf("store restore failed: %w", err)
	}

	return nil
}

// LoadCheckpoint loads state from persistent checkpoint.
// Only available if checkpointer was configured.
func (m *Manager) LoadCheckpoint(ctx context.Context) error {
	if m.checkpointer == nil {
		return fmt.Errorf("checkpointer not configured")
	}
	if m.checkpointRunID == "" {
		return fmt.Errorf("checkpoint run ID not configured")
	}

	cp, err := m.checkpointer.Load(ctx, m.checkpointRunID)
	if err != nil {
		return fmt.Errorf("checkpoint load failed: %w", err)
	}
	if cp == nil {
		// No checkpoint exists, this is fine
		return nil
	}

	// Restore state from checkpoint
	if err := m.channels.Restore(ctx, cp.State); err != nil {
		return fmt.Errorf("channel restore from checkpoint failed: %w", err)
	}

	if err := m.store.Restore(ctx, cp.State); err != nil {
		return fmt.Errorf("store restore from checkpoint failed: %w", err)
	}

	return nil
}

// ListSnapshots returns all in-memory snapshot IDs (newest first).
func (m *Manager) ListSnapshots() []string {
	return m.snapshots.ListSnapshots()
}

// DeleteSnapshot removes an in-memory snapshot.
func (m *Manager) DeleteSnapshot(snapshotID string) error {
	return m.snapshots.DeleteSnapshot(snapshotID)
}

// RegisteredKeys returns all registered key names.
func (m *Manager) RegisteredKeys() []string {
	return m.types.RegisteredKeys()
}

// Close closes the manager and releases resources.
func (m *Manager) Close() error {
	if err := m.store.Close(); err != nil {
		return fmt.Errorf("store close failed: %w", err)
	}
	return nil
}

// CreateReadView creates a read-only view of the current state for BSP execution.
// This allows nodes to read state concurrently without mutations.
func (m *Manager) CreateReadView(ctx context.Context) (*ReadView, error) {
	// Take a snapshot for concurrent reads
	versionedSnapshot, err := m.Snapshot(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}

	// Convert VersionedSnapshot to Snapshot for ReadView
	stateSnapshot := &Snapshot{
		data:    versionedSnapshot.Data,
		version: 0, // Version tracking handled by SnapshotManager
	}

	return NewReadView(stateSnapshot), nil
}

// convertMetadata converts map[string]string to map[string]any for checkpoint.
func convertMetadata(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
