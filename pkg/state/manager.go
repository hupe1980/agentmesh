package state

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/state/internal/channel"
)

// Manager is the concrete state manager implementation.
// It coordinates:
// - Store: Pluggable storage backend (memory, Redis, DynamoDB, etc.)
// - ChannelRegistry: Channel-based storage with semantic behaviors
// - SnapshotManager: In-memory versioning for rollback
// - Checkpointer: Persistent checkpointing (optional)
//
// Type safety is enforced at compile-time through generic wrapper functions
// (RegisterKey, Get, Set, Append, GetList) without runtime reflection.
//
// Architecture: Channels ARE the storage layer, Keys are type-safe accessors.
type Manager struct {
	mu              sync.RWMutex
	store           Store
	channels        *ChannelRegistry
	registeredKeys  map[string]keyInfo          // Tracks registered keys without reflection
	managedValues   map[string]*ManagedValueAny // Ephemeral runtime state (NOT checkpointed)
	snapshots       *SnapshotManager
	checkpointer    checkpoint.Checkpointer
	checkpointRunID string
}

// keyInfo tracks registered key metadata without requiring type information.
type keyInfo struct {
	name        string
	isList      bool
	isAggregate bool
	maxSize     int // For list keys only
}

// ManagerOption configures manager behavior.
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

// WithMaxSnapshotsLimit limits in-memory snapshot retention.
func WithMaxSnapshotsLimit(maxSnapshots int) ManagerOption {
	return func(m *Manager) {
		m.snapshots = NewSnapshotManager(WithMaxSnapshots(maxSnapshots))
	}
}

// NewManager creates a new unified state manager.
// Default configuration:
// - MemoryStore for storage
// - No checkpointing
// - Unlimited in-memory snapshots
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		store:          NewMemoryStore(),
		channels:       NewChannelRegistry(),
		registeredKeys: make(map[string]keyInfo),
		managedValues:  make(map[string]*ManagedValueAny),
		snapshots:      NewSnapshotManager(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// RegisterKey registers a Key[T] with the manager.
// This creates the corresponding LastValueChannel.
// If the key is already registered, this is a no-op (idempotent).
// Type safety is enforced at compile-time through the Key[T] parameter.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	state.RegisterKey(mgr, counterKey)
func RegisterKey[T any](m *Manager, key Key[T]) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already registered (idempotent)
	if _, exists := m.registeredKeys[key.name]; exists {
		return nil
	}

	// Create LastValueChannel for scalar keys
	ch := channel.NewLastValueChannel(key.name)
	if err := m.channels.RegisterChannel(key.name, ch, LastValueBehavior); err != nil {
		return fmt.Errorf("channel registration failed: %w", err)
	}

	// Initialize with default value
	ctx := context.Background()
	if err := ch.Write(ctx, key.zero); err != nil {
		return fmt.Errorf("failed to initialize channel: %w", err)
	}

	// Track registration
	m.registeredKeys[key.name] = keyInfo{
		name:   key.name,
		isList: false,
	}

	return nil
}

// RegisterListKey registers a ListKey[T] with the manager.
// This creates the corresponding TopicChannel with maxSize limit.
// If the key is already registered, this is a no-op (idempotent).
// Type safety is enforced at compile-time through the ListKey[T] parameter.
//
// Example:
//
//	messagesKey := NewListKey[string]("messages", 100)
//	state.RegisterListKey(mgr, messagesKey)
func RegisterListKey[T any](m *Manager, key ListKey[T]) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already registered (idempotent)
	if _, exists := m.registeredKeys[key.name]; exists {
		return nil
	}

	// Create TopicChannel for list keys (append semantics)
	ch := channel.NewTopicChannel(key.name, key.maxSize)
	if err := m.channels.RegisterChannel(key.name, ch, TopicBehavior); err != nil {
		return fmt.Errorf("channel registration failed: %w", err)
	}

	// Track registration
	m.registeredKeys[key.name] = keyInfo{
		name:    key.name,
		isList:  true,
		maxSize: key.maxSize,
	}

	return nil
}

// RegisterAggregateKey registers a Key[T] with aggregation semantics.
// This creates an AggregateChannel that combines values using the provided aggregator.
// If the key is already registered, this is a no-op (idempotent).
// Type safety is enforced at compile-time through the Key[T] parameter.
//
// Use this for global coordination patterns:
//   - Counters (SumAggregator)
//   - Maximum/minimum tracking (MaxAggregator, MinAggregator)
//   - Statistical computations (AvgAggregator, VarianceAggregator)
//   - Convergence detection (AllTrueAggregator)
//
// Example:
//
//	totalCostKey := NewKey[float64]("total_cost", 0.0)
//	state.RegisterAggregateKey(mgr, totalCostKey, &SumAggregator{})
//
//	// Nodes contribute via normal Updates:
//	return state.Updates{totalCostKey.Name(): 42.0}, nil
func RegisterAggregateKey[T any](m *Manager, key Key[T], aggregator channel.Aggregator) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already registered (idempotent)
	if _, exists := m.registeredKeys[key.name]; exists {
		return nil
	}

	// Create AggregateChannel with the provided aggregator
	ch := channel.NewAggregateChannel(key.name, aggregator)
	if err := m.channels.RegisterChannel(key.name, ch, AggregateBehavior); err != nil {
		return fmt.Errorf("channel registration failed: %w", err)
	}

	// Track registration (mark as aggregate type)
	m.registeredKeys[key.name] = keyInfo{
		name:        key.name,
		isList:      false,
		isAggregate: true,
	}

	return nil
}

// Get retrieves a typed value from state.
// If the key doesn't exist, returns the key's default value.
// Type safety is enforced at compile-time through the Key[T] parameter.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	value := state.Get(ctx, manager, counterKey)
func Get[T any](ctx context.Context, m *Manager, key Key[T]) (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Read from channel
	value, err := m.channels.GetChannelValue(ctx, key.name)
	if err != nil {
		return key.zero, err
	}

	// Handle nil/empty channel
	if value == nil {
		return key.zero, nil
	}

	// Type assertion (safe because RegisterKey[T] ensures type consistency)
	typed, ok := value.(T)
	if !ok {
		return key.zero, fmt.Errorf("type mismatch for key %q: expected %T, got %T", key.name, key.zero, value)
	}

	return typed, nil
}

// Set updates a typed value in state.
// Type safety is enforced at compile-time through the Key[T] parameter.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	err := state.Set(ctx, manager, counterKey, 42)
func Set[T any](ctx context.Context, m *Manager, key Key[T], value T) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

// Append adds a value to a list.
// Type safety is enforced at compile-time through the ListKey[T] parameter.
//
// Example:
//
//	messagesKey := NewListKey[string]("messages", 100)
//	err := state.Append(ctx, manager, messagesKey, "Hello")
func Append[T any](ctx context.Context, m *Manager, key ListKey[T], value T) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Write to channel (TopicChannel handles appending)
	if err := m.channels.WriteValue(ctx, key.name, value); err != nil {
		return fmt.Errorf("channel write failed: %w", err)
	}

	// For persistence, get current list, append, then store
	currentList, err := m.channels.GetChannelValue(ctx, key.name)
	if err != nil {
		return fmt.Errorf("failed to get current list: %w", err)
	}

	var updatedList []T
	if currentList != nil {
		if existing, ok := currentList.([]T); ok {
			existing = append(existing, value)
			updatedList = existing
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

// GetList retrieves all list values with compile-time type safety.
// Returns []T or an error if not found/mistyped.
//
// Example:
//
//	messagesKey := NewListKey[string]("messages", 100)
//	messages, err := state.GetList(ctx, manager, messagesKey)
func GetList[T any](ctx context.Context, m *Manager, key ListKey[T]) ([]T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Read from channel
	value, err := m.channels.GetChannelValue(ctx, key.name)
	if err != nil {
		return nil, err
	}

	if value == nil {
		return []T{}, nil
	}

	// Type assertion: []any -> []T
	items, ok := value.([]T)
	if !ok {
		return nil, fmt.Errorf("type mismatch for key %q: expected []%T, got %T", key.name, *new(T), value)
	}

	return items, nil
}

// GetChannel retrieves the underlying channel for a key.
// Useful for advanced operations like direct channel manipulation.
//
// Example:
//
//	ch := manager.GetChannel("messages")
//	value, err := ch.Read(ctx)
func (m *Manager) GetChannel(name string) channel.Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels.GetChannel(name)
}

// ApplyUpdates applies a map of updates to the manager.
// For registered list keys, values are appended. For regular keys, values are set/replaced.
// This is a convenience method for batch updates during graph execution.
func (m *Manager) ApplyUpdates(ctx context.Context, updates Updates) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if updates == nil {
		return nil
	}

	for key, value := range updates {
		// Check for deletion marker
		if _, isDelete := value.(deleteMarker); isDelete {
			// Delete the key from channels
			if err := m.channels.DeleteChannel(key); err != nil {
				return fmt.Errorf("failed to delete key %q: %w", key, err)
			}
			// Delete from store
			if err := m.store.Delete(ctx, key); err != nil {
				return fmt.Errorf("failed to delete key %q from store: %w", key, err)
			}
			// Remove from registered keys
			delete(m.registeredKeys, key)
			continue
		}

		// Check if this is a registered list key
		info, exists := m.registeredKeys[key]
		isListKey := exists && info.isList

		if isListKey {
			// For list keys, append the value
			// Note: value might be a single item or a slice
			if err := m.channels.WriteValue(ctx, key, value); err != nil {
				return fmt.Errorf("failed to append to key %q: %w", key, err)
			}
		} else {
			// For regular keys, set/replace the value
			if err := m.channels.WriteValue(ctx, key, value); err != nil {
				return fmt.Errorf("failed to set key %q: %w", key, err)
			}
		}
	}

	return nil
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.registeredKeys))
	for name := range m.registeredKeys {
		keys = append(keys, name)
	}
	return keys
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

// RegisterManagedValue registers a type-safe managed value for ephemeral runtime state.
// Managed values are NOT included in checkpoints - they are runtime-only state.
//
// Use cases:
//   - Configuration stores (runtime config that shouldn't be persisted)
//   - Session state (user sessions, auth tokens, connection pools)
//   - Metrics collectors (runtime metrics that don't need persistence)
//   - Resource handles (database connections, file handles)
//
// Example:
//
//	// Create a managed value for runtime configuration
//	configMV := state.NewManagedValue[*RuntimeConfig]("runtime_config")
//	mgr.RegisterManagedValue(configMV)
//
//	// Set the configuration (typically done at runtime initialization)
//	config := &RuntimeConfig{APIKey: "secret", Timeout: 30}
//	configMV.Set(ctx, config)
//
//	// Access from nodes via GetManagedValue
//	config, err := state.GetManagedValue[*RuntimeConfig](mgr, ctx, "runtime_config")
func RegisterManagedValue[T any](m *Manager, mv ManagedValue[T]) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := mv.Name()

	// Check if already registered
	if _, exists := m.managedValues[name]; exists {
		return fmt.Errorf("managed value %q already registered", name)
	}

	// Wrap for type-erased storage
	m.managedValues[name] = WrapManagedValue(mv)

	return nil
}

// GetManagedValue retrieves a type-safe managed value from the manager.
// Returns an error if the managed value doesn't exist or type assertion fails.
//
// Example:
//
//	// In a node's Compute function
//	func (n *MyNode) Compute(ctx context.Context, s state.State) (state.Updates, error) {
//	    config, err := state.GetManagedValue[*RuntimeConfig](ctx, mgr, "runtime_config")
//	    if err != nil {
//	        return nil, err
//	    }
//	    // Use config.APIKey, config.Timeout, etc.
//	}
func GetManagedValue[T any](ctx context.Context, m *Manager, name string) (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var zero T

	mv, exists := m.managedValues[name]
	if !exists {
		return zero, fmt.Errorf("managed value %q not found", name)
	}

	value, err := mv.Get(ctx)
	if err != nil {
		return zero, err
	}

	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("managed value %q has wrong type: expected %T, got %T", name, zero, value)
	}

	return typed, nil
}

// SetManagedValue updates a managed value (runtime operation, not from nodes).
// This is typically called during runtime initialization or by the graph executor,
// not from within node Compute functions.
//
// Example:
//
//	// Update configuration at runtime
//	newConfig := &RuntimeConfig{APIKey: "new-secret", Timeout: 60}
//	err := state.SetManagedValue(ctx, mgr, "runtime_config", newConfig)
func SetManagedValue[T any](ctx context.Context, m *Manager, name string, value T) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mv, exists := m.managedValues[name]
	if !exists {
		return fmt.Errorf("managed value %q not found", name)
	}

	return mv.Set(ctx, value)
}

// GetManagedValueNames returns all registered managed value names.
// Useful for debugging and introspection.
func (m *Manager) GetManagedValueNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.managedValues))
	for name := range m.managedValues {
		names = append(names, name)
	}

	return names
}
