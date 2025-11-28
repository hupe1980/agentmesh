package state

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/state/internal/channel"
)

// ManagerBuilder provides a type-safe way to construct a Manager during the setup phase.
// Once Build() is called, the resulting Manager is immutable for schema changes.
// This enforces compile-time separation between setup and runtime phases.
//
// Example:
//
//	builder := state.NewManagerBuilder()
//	builder.RegisterKey(counterKey)
//	builder.RegisterListKey(messagesKey)
//	manager := builder.Build()  // Manager is now frozen
type ManagerBuilder struct {
	store           Store
	checkpointer    checkpoint.Checkpointer
	checkpointRunID string
	maxSnapshots    int

	channels       *ChannelRegistry
	registeredKeys map[string]keyInfo
	managedValues  map[string]*ManagedValueAny
}

// NewManagerBuilder creates a new manager builder with default configuration.
func NewManagerBuilder(opts ...ManagerOption) *ManagerBuilder {
	builder := &ManagerBuilder{
		store:          NewMemoryStore(),
		channels:       NewChannelRegistry(),
		registeredKeys: make(map[string]keyInfo),
		managedValues:  make(map[string]*ManagedValueAny),
		maxSnapshots:   0, // Unlimited by default
	}

	// Apply options to configure the builder
	tempManager := &Manager{
		store:           builder.store,
		checkpointer:    builder.checkpointer,
		checkpointRunID: builder.checkpointRunID,
	}

	for _, opt := range opts {
		opt(tempManager)
	}

	// Extract configured values back
	builder.store = tempManager.store
	builder.checkpointer = tempManager.checkpointer
	builder.checkpointRunID = tempManager.checkpointRunID

	// Extract maxSnapshots if it was set via WithMaxSnapshotsLimit
	if tempManager.snapshots != nil {
		builder.maxSnapshots = tempManager.snapshots.maxSnapshots
	}

	return builder
}

// RegisterKey registers a Key[T] with the builder.
// This creates the corresponding LastValueChannel.
// If the key is already registered, this is a no-op (idempotent).
// Type safety is enforced at compile-time through the Key[T] parameter.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	builder.RegisterKey(counterKey)
func RegisterKey[T any](b *ManagerBuilder, key Key[T]) error {
	// Check if already registered (idempotent)
	if _, exists := b.registeredKeys[key.name]; exists {
		return nil
	}

	// Create and initialize channel
	ch := channel.NewLastValueChannel(key.name)
	ctx := context.Background()
	if err := ch.Write(ctx, key.zero); err != nil {
		return fmt.Errorf("failed to initialize channel: %w", err)
	}

	// Register the pre-initialized channel
	if err := b.channels.RegisterChannel(key.name, ch, LastValueBehavior); err != nil {
		return fmt.Errorf("channel registration failed: %w", err)
	}

	// Track registration
	info := keyInfo{
		name:   key.name,
		isList: false,
	}
	b.registeredKeys[key.name] = info

	return nil
}

// RegisterListKey registers a ListKey[T] with the builder.
// This creates the corresponding TopicChannel with maxSize limit.
// If the key is already registered, this is a no-op (idempotent).
// Type safety is enforced at compile-time through the ListKey[T] parameter.
//
// Example:
//
//	messagesKey := NewListKey[string]("messages", 100)
//	builder.RegisterListKey(messagesKey)
func RegisterListKey[T any](b *ManagerBuilder, key ListKey[T]) error {
	// Check if already registered (idempotent)
	if _, exists := b.registeredKeys[key.name]; exists {
		return nil
	}

	// Create TopicChannel for list keys (append semantics)
	ch := channel.NewTopicChannel(key.name, key.maxSize)
	if err := b.channels.RegisterChannel(key.name, ch, TopicBehavior); err != nil {
		return fmt.Errorf("channel registration failed: %w", err)
	}

	// Track registration
	info := keyInfo{
		name:    key.name,
		isList:  true,
		maxSize: key.maxSize,
	}
	b.registeredKeys[key.name] = info

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
//	builder.RegisterAggregateKey(totalCostKey, &SumAggregator{})
//
//	// Nodes contribute via normal Updates:
//	return state.Updates{totalCostKey.Name(): 42.0}, nil
func RegisterAggregateKey[T any](b *ManagerBuilder, key Key[T], aggregator channel.Aggregator) error {
	// Check if already registered (idempotent)
	if _, exists := b.registeredKeys[key.name]; exists {
		return nil
	}

	// Create AggregateChannel with the provided aggregator
	ch := channel.NewAggregateChannel(key.name, aggregator)
	if err := b.channels.RegisterChannel(key.name, ch, AggregateBehavior); err != nil {
		return fmt.Errorf("channel registration failed: %w", err)
	}

	// Track registration
	info := keyInfo{
		name:        key.name,
		isAggregate: true,
	}
	b.registeredKeys[key.name] = info

	return nil
}

// RegisterManagedValue registers a ManagedValue[T] with the builder.
// This allows automatic lifecycle management during graph execution.
//
// Example:
//
//	dbConn := state.NewManagedValueWithDefault("db", &DatabaseConnection{...})
//	builder.RegisterManagedValue(dbConn)
func RegisterManagedValue[T any](b *ManagerBuilder, mv ManagedValue[T]) error {
	name := mv.Name()
	if _, exists := b.managedValues[name]; exists {
		return fmt.Errorf("managed value with key %q already registered", name)
	}

	// Wrap the typed ManagedValue into type-erased form
	b.managedValues[name] = WrapManagedValue(mv)

	return nil
}

// WithStore configures a custom store for the builder.
func (b *ManagerBuilder) WithStore(store Store) *ManagerBuilder {
	b.store = store
	return b
}

// WithCheckpointer configures a checkpointer for the builder.
func (b *ManagerBuilder) WithCheckpointer(checkpointer checkpoint.Checkpointer, runID string) *ManagerBuilder {
	b.checkpointer = checkpointer
	b.checkpointRunID = runID
	return b
}

// WithMaxSnapshotsLimit configures the maximum number of snapshots to retain.
func (b *ManagerBuilder) WithMaxSnapshotsLimit(limit int) *ManagerBuilder {
	b.maxSnapshots = limit
	return b
}

// Build creates a frozen Manager from the builder configuration.
// After calling Build(), the Manager's schema cannot be modified.
// This enforces immutability at the type system level.
//
// Example:
//
//	builder := state.NewManagerBuilder()
//	builder.RegisterKey(counterKey)
//	mgr := builder.Build()  // Manager is now frozen
func (b *ManagerBuilder) Build() *Manager {
	// Create snapshot manager with configured limit
	var snapshotMgr *SnapshotManager
	if b.maxSnapshots > 0 {
		snapshotMgr = NewSnapshotManager(WithMaxSnapshots(b.maxSnapshots))
	} else {
		snapshotMgr = NewSnapshotManager()
	}

	return &Manager{
		store:           b.store,
		checkpointer:    b.checkpointer,
		checkpointRunID: b.checkpointRunID,
		channels:        b.channels,
		registeredKeys:  b.registeredKeys,
		managedValues:   b.managedValues,
		frozen:          true, // Always frozen after Build()
		snapshots:       snapshotMgr,
	}
}
