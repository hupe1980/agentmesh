package state

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// State Management Architecture
//
// This file implements the state management layer following the Interface Segregation
// Principle. Previously, StateManager was a monolithic interface with 20+ methods,
// making it difficult to test and violating the Single Responsibility Principle.
//
// Current Design (Refactored):
//
// 1. Reader (4 methods) - Read-only access for node execution
//    Defined in pkg/state to break circular dependencies with pkg/tool
//    Use case: Pass to node RunFunc to prevent direct state mutations
//
// 2. Writer (2 methods) - Extends Reader with write capabilities
//    Defined in pkg/state to break circular dependencies with pkg/tool
//    Use case: Pass to nodes that need to update state or contribute to aggregates
//
// 3. ChannelManager (7 methods) - Channel lifecycle management
//    Use case: Internal graph runtime for managing channel updates
//
// 4. AggregateManager (5 methods) - Cross-node aggregate coordination
//    Use case: Runtime coordination of global aggregates across supersteps
//
// 5. CheckpointManager (3 methods) - State persistence and restoration
//    Use case: Configure and manage checkpoint backends
//
// 6. StateManager - Composed interface that brings all concerns together
//    Use case: Internal use by graphRuntime for full state coordination
//
// Benefits:
// - Small, focused interfaces are easier to mock in tests
// - Clear separation of concerns (read, write, channels, aggregates, checkpoints)
// - Clients depend only on the methods they actually use
// - Better documentation with focused interface docs
// - Enables partial implementations for specialized use cases
//
// Implementation:
// - State struct implements all interfaces
// - No breaking changes to State struct itself
// - Nodes receive Reader or Writer (not full StateManager)
// - Runtime uses StateManager for full control

// =============================================================================
// Focused State Interfaces (Interface Segregation Principle)
// =============================================================================

// ChannelManager handles channel lifecycle and batch operations.
// Separated from Reader/Writer for clear responsibility boundaries.
//
// Use Case: Internal graph runtime management of channels.
type ChannelManager interface {
	// AddChannel registers a new channel in the state.
	AddChannel(ch channel.Channel)

	// GetChannel retrieves a channel by name.
	GetChannel(name string) (channel.Channel, bool)

	// Set updates or creates a channel value.
	Set(key string, value any) error

	// UpdateChannel writes a value to a specific channel.
	UpdateChannel(ctx context.Context, name string, value any) error

	// UpdateChannels batch-updates multiple channels atomically.
	UpdateChannels(ctx context.Context, updates map[string]any) error

	// AddMessages appends messages to the "messages" channel.
	AddMessages(messages []ExecutionResult)

	// ApplyUpdates applies channel updates and messages in a single operation.
	ApplyUpdates(values map[string]any, messages []ExecutionResult)
}

// AggregateManager handles aggregate value management for cross-node reductions.
// Provides both read access and configuration for aggregation logic.
//
// Use Case: Runtime coordination of global aggregates across supersteps.
type AggregateManager interface {
	// GetAggregate retrieves the current value of a named aggregate.
	GetAggregate(name string) any

	// GetAggregatesSnapshot returns a read-only snapshot of all aggregates.
	GetAggregatesSnapshot() map[string]any

	// SetAggregates replaces all aggregates with the provided map.
	SetAggregates(aggregates map[string]any)

	// SetAggregateFn configures the function used to combine aggregate values.
	SetAggregateFn(fn func(string, any) error)

	// RecordAggregation records a value for aggregation.
	RecordAggregation(name string, value any) error
}

// CheckpointManager handles persistence and restoration of graph state.
// Decoupled from state operations for pluggable checkpoint backends.
//
// Use Case: Configure and manage checkpoint persistence.
type CheckpointManager interface {
	// SaveCheckpoint persists current state to configured backend.
	SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error

	// LoadCheckpoint restores state from a previous checkpoint.
	LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)

	// SetCheckpointer configures the checkpoint backend.
	SetCheckpointer(checkpointer checkpoint.Checkpointer)
}

// =============================================================================
// Composed StateManager Interface
// =============================================================================

// StateManager is the complete interface composing all state management concerns.
// It provides full control over channels, checkpoints, aggregates, and state access.
//
// Interface Composition:
//   - Reader: Read-only state access
//   - ChannelManager: Channel lifecycle and updates
//   - AggregateManager: Cross-node aggregate management
//   - CheckpointManager: State persistence and restoration
//
// Additional Capabilities:
//   - Version tracking for checkpoint integrity
//   - State snapshots for debugging
//   - State cloning for independent execution contexts
//
// Design Goals:
//   - Interface Segregation: Clients depend only on methods they use
//   - Single Responsibility: Each sub-interface has one clear purpose
//   - Testability: Easy to mock small, focused interfaces
//   - Extensibility: Can implement subsets for specialized use cases
//
// Use Case: Internal use by graphRuntime for full state coordination.
//
//nolint:revive // StateManager is an established API name
type StateManager interface {
	Writer // Extends Reader with write capabilities (Set, Aggregate)
	ChannelManager
	AggregateManager
	CheckpointManager

	// Version returns the current state version (monotonic counter).
	Version() uint64

	// Snapshot returns a complete snapshot of all channel values.
	Snapshot() map[string]any

	// Clone creates an independent copy of the state manager.
	Clone() StateManager
}

// =============================================================================
// ChannelState - Primary StateManager Implementation
// =============================================================================

// ChannelState manages channel-based state for graph execution.
// This is the concrete implementation of the StateManager interface.
//
// It manages data flow through typed channels with thread-safe access.
//
// ChannelState serves as both:
// - The user-facing API for building graphs (via NewStateManager, Graph.State)
// - The runtime state during execution (no conversion needed)
//
// This is the ONLY StateManager implementation in v2.0+ (Option A architecture).
//
// Thread Safety:
// - Channel operations use per-channel locks (via Set and individual channels)
// - Aggregates use separate aggregatesMu lock
// - Checkpointer uses checkpointerMu for safe concurrent access
// This design eliminates global lock contention for better concurrent performance.
//
// Performance Optimization (Phase 3):
// - Lazy copy-on-write for aggregate snapshots
// - Version tracking to invalidate cached snapshots
// - Avoids full map copy on every GetAggregatesSnapshot() call
type ChannelState struct {
	channels       *channel.Set
	aggregates     map[string]any
	aggregateFn    func(string, any) error
	aggregatesMu   sync.RWMutex
	checkpointer   checkpoint.Checkpointer
	checkpointerMu sync.RWMutex

	// State versioning for checkpoint integrity
	version   uint64     // Monotonic version counter, incremented on every state mutation
	versionMu sync.Mutex // Protects version counter

	// Aggregate snapshot caching
	aggregateCache   map[string]any // Cached snapshot of aggregates
	aggregateVersion uint64         // Version counter to invalidate cache
	cachedVersion    uint64         // Version of current cache
}

// NewStateManager creates a new StateManager with the default ChannelState implementation.
// It automatically creates a standard "messages" channel (Topic with maxMessages limit).
// This is the recommended way to create a state manager for graph execution.
//
// Returns an error if maxMessages is negative.
func NewStateManager(maxMessages int) (StateManager, error) {
	if maxMessages < 0 {
		return nil, fmt.Errorf("maxMessages must be non-negative, got %d", maxMessages)
	}
	channels := channel.NewSet()
	channels.Add(channel.NewTopicChannel("messages", maxMessages))
	return &ChannelState{
		channels:   channels,
		aggregates: make(map[string]any),
	}, nil
}

// NewChannelState creates a new channel-based graph state.
// Returns a concrete *ChannelState for cases requiring direct access to the implementation.
// For most use cases, prefer NewStateManager() which returns the StateManager interface.
//
// Returns an error if maxMessages is negative.
func NewChannelState(maxMessages int) (*ChannelState, error) {
	if maxMessages < 0 {
		return nil, fmt.Errorf("maxMessages must be non-negative, got %d", maxMessages)
	}
	channels := channel.NewSet()
	channels.Add(channel.NewTopicChannel("messages", maxMessages))
	return &ChannelState{
		channels:   channels,
		aggregates: make(map[string]any),
	}, nil
}

// =============================================================================
// ChannelState - State Read Methods
// =============================================================================

// Get retrieves a value from the state by key.
func (s *ChannelState) Get(key string) any {
	// No lock needed - Set.Get() and Channel.Read() handle their own locking
	ch, ok := s.channels.Get(key)
	if !ok {
		return nil
	}
	val, err := ch.Read(context.Background())
	if err != nil {
		return nil
	}
	return val
}

// GetAll returns all state values as a map.
func (s *ChannelState) GetAll() map[string]any {
	// No lock needed - Set.ReadAll() handles its own locking
	values, err := s.channels.ReadAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

// MessagesSnapshot returns a copy of current message history with metadata.
// Returns []ExecutionResult containing messages with node/timestamp metadata.
func (s *ChannelState) MessagesSnapshot() []ExecutionResult {
	ch, ok := s.GetChannel("messages")
	if !ok {
		return nil
	}

	val, err := ch.Read(context.Background())
	if err != nil || val == nil {
		return nil
	}

	// Convert []any to []ExecutionResult
	values, ok := val.([]any)
	if !ok || len(values) == 0 {
		return nil
	}

	results := make([]ExecutionResult, 0, len(values))
	for _, v := range values {
		if evt, ok := v.(ExecutionResult); ok {
			results = append(results, evt)
		}
	}
	return results
}

// =============================================================================
// ChannelState - Type-Safe Accessors (Prevent Runtime Panics)
// =============================================================================

// GetString retrieves a string value from state with type checking.
// Returns an error if the key doesn't exist or the value is not a string.
func (s *ChannelState) GetString(key string) (string, error) {
	val := s.Get(key)
	if val == nil {
		return "", fmt.Errorf("key %q not found in state", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("key %q has type %T, expected string", key, val)
	}
	return str, nil
}

// GetInt retrieves an integer value from state with type checking.
// Returns an error if the key doesn't exist or the value is not an int.
func (s *ChannelState) GetInt(key string) (int, error) {
	val := s.Get(key)
	if val == nil {
		return 0, fmt.Errorf("key %q not found in state", key)
	}
	i, ok := val.(int)
	if !ok {
		return 0, fmt.Errorf("key %q has type %T, expected int", key, val)
	}
	return i, nil
}

// GetInt64 retrieves an int64 value from state with type checking.
// Returns an error if the key doesn't exist or the value is not an int64.
func (s *ChannelState) GetInt64(key string) (int64, error) {
	val := s.Get(key)
	if val == nil {
		return 0, fmt.Errorf("key %q not found in state", key)
	}
	i64, ok := val.(int64)
	if !ok {
		return 0, fmt.Errorf("key %q has type %T, expected int64", key, val)
	}
	return i64, nil
}

// GetFloat64 retrieves a float64 value from state with type checking.
// Returns an error if the key doesn't exist or the value is not a float64.
func (s *ChannelState) GetFloat64(key string) (float64, error) {
	val := s.Get(key)
	if val == nil {
		return 0, fmt.Errorf("key %q not found in state", key)
	}
	f64, ok := val.(float64)
	if !ok {
		return 0, fmt.Errorf("key %q has type %T, expected float64", key, val)
	}
	return f64, nil
}

// GetBool retrieves a boolean value from state with type checking.
// Returns an error if the key doesn't exist or the value is not a bool.
func (s *ChannelState) GetBool(key string) (bool, error) {
	val := s.Get(key)
	if val == nil {
		return false, fmt.Errorf("key %q not found in state", key)
	}
	b, ok := val.(bool)
	if !ok {
		return false, fmt.Errorf("key %q has type %T, expected bool", key, val)
	}
	return b, nil
}

// GetSlice retrieves a slice value from state with type checking.
// Returns an error if the key doesn't exist or the value is not a slice.
func (s *ChannelState) GetSlice(key string) ([]any, error) {
	val := s.Get(key)
	if val == nil {
		return nil, fmt.Errorf("key %q not found in state", key)
	}
	slice, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("key %q has type %T, expected []any", key, val)
	}
	return slice, nil
}

// GetMap retrieves a map value from state with type checking.
// Returns an error if the key doesn't exist or the value is not a map[string]any.
func (s *ChannelState) GetMap(key string) (map[string]any, error) {
	val := s.Get(key)
	if val == nil {
		return nil, fmt.Errorf("key %q not found in state", key)
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("key %q has type %T, expected map[string]any", key, val)
	}
	return m, nil
}

// =============================================================================
// State - Channel Management
// =============================================================================

// AddChannel registers a new channel in the state.
func (s *ChannelState) AddChannel(ch channel.Channel) {
	// No lock needed - Set.Add() handles its own locking
	s.channels.Add(ch)
}

// GetChannel retrieves a channel by name.
func (s *ChannelState) GetChannel(name string) (channel.Channel, bool) {
	// No lock needed - Set.Get() handles its own locking
	return s.channels.Get(name)
}

// UpdateChannel writes a value to a specific channel.
func (s *ChannelState) UpdateChannel(ctx context.Context, name string, value any) error {
	// No lock needed - Set.Get() and Channel.Write() handle their own locking
	ch, ok := s.channels.Get(name)
	if !ok {
		return nil // Silently ignore unknown channels
	}
	return ch.Write(ctx, value)
}

// UpdateChannels batch-updates multiple channels.
func (s *ChannelState) UpdateChannels(ctx context.Context, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	// No lock needed - Set.Get() and Channel.Write() handle their own locking
	for name, value := range updates {
		ch, ok := s.channels.Get(name)
		if !ok {
			continue // Skip unknown channels
		}
		if err := ch.Write(ctx, value); err != nil {
			return fmt.Errorf("failed to write to channel %q: %w", name, err)
		}
	}

	// Increment version counter after successful mutations
	s.incrementVersion()

	return nil
}

// =============================================================================
// State - Aggregate Management
// =============================================================================

// GetAggregate retrieves the current value of a named aggregate.
func (s *ChannelState) GetAggregate(name string) any {
	s.aggregatesMu.RLock()
	defer s.aggregatesMu.RUnlock()
	return s.aggregates[name]
}

// GetAggregatesSnapshot returns a read-only snapshot of all aggregates.
func (s *ChannelState) GetAggregatesSnapshot() map[string]any {
	s.aggregatesMu.RLock()

	// Fast path: return cached snapshot if version matches
	if s.cachedVersion == s.aggregateVersion && s.aggregateCache != nil {
		s.aggregatesMu.RUnlock()
		return s.aggregateCache
	}

	// Slow path: cache miss or version mismatch - need to create snapshot
	if len(s.aggregates) == 0 {
		s.aggregatesMu.RUnlock()
		return nil
	}

	// Upgrade to write lock to update cache
	s.aggregatesMu.RUnlock()
	s.aggregatesMu.Lock()
	defer s.aggregatesMu.Unlock()

	// Double-check: another goroutine might have updated the cache
	if s.cachedVersion == s.aggregateVersion && s.aggregateCache != nil {
		return s.aggregateCache
	}

	// Create snapshot and cache it
	snapshot := make(map[string]any, len(s.aggregates))
	for k, v := range s.aggregates {
		snapshot[k] = v
	}
	s.aggregateCache = snapshot
	s.cachedVersion = s.aggregateVersion

	return snapshot
}

// AggregatesSnapshot is an alias for GetAggregatesSnapshot for backward compatibility.
func (s *ChannelState) AggregatesSnapshot() map[string]any {
	return s.GetAggregatesSnapshot()
}

// =============================================================================
// State - Version Management
// =============================================================================

// incrementVersion atomically increments the state version counter.
// Called after any state mutation to track state evolution for checkpoint integrity.
func (s *ChannelState) incrementVersion() {
	s.versionMu.Lock()
	s.version++
	s.versionMu.Unlock()
}

// Version returns the current state version.
// This monotonic counter increases with every state mutation.
func (s *ChannelState) Version() uint64 {
	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	return s.version
}

// SetVersion explicitly sets the version (used during checkpoint restore).
func (s *ChannelState) SetVersion(v uint64) {
	s.versionMu.Lock()
	s.version = v
	s.versionMu.Unlock()
}

// =============================================================================
// State - Aggregate Management (continued)
// =============================================================================

// SetAggregates replaces all aggregates with the provided map.
func (s *ChannelState) SetAggregates(aggregates map[string]any) {
	s.aggregatesMu.Lock()
	defer s.aggregatesMu.Unlock()

	// Direct map replacement - much faster than delete + copy loop
	// Old implementation: O(old_size + new_size) with lock held
	// New implementation: O(1) pointer assignment + version increment
	s.aggregates = aggregates
	s.aggregateVersion++ // Invalidate cached snapshot

	// Increment global version counter
	s.incrementVersion()
}

// SetAggregateFn sets the aggregation function used to combine values.
func (s *ChannelState) SetAggregateFn(fn func(string, any) error) {
	s.aggregatesMu.Lock()
	defer s.aggregatesMu.Unlock()
	s.aggregateFn = fn
}

// RecordAggregation records a value for aggregation using the configured aggregation function.
func (s *ChannelState) RecordAggregation(name string, value any) error {
	s.aggregatesMu.RLock()
	fn := s.aggregateFn
	s.aggregatesMu.RUnlock()

	if fn == nil {
		return ErrAggregatorsNotConfigured
	}
	return fn(name, value)
}

// Aggregate is an alias for RecordAggregation for backward compatibility.
func (s *ChannelState) Aggregate(name string, value any) error {
	return s.RecordAggregation(name, value)
}

// =============================================================================
// State - Checkpoint Management
// =============================================================================

// SaveCheckpoint saves the current state to the configured checkpointer.
func (s *ChannelState) SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error {
	s.checkpointerMu.RLock()
	checkpointer := s.checkpointer
	s.checkpointerMu.RUnlock()

	if checkpointer == nil {
		return nil // No checkpointer configured
	}

	snapshot := s.Snapshot()
	return checkpointer.Save(ctx, &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: superstep,
		State:     snapshot,
		Metadata:  metadata,
	})
}

// LoadCheckpoint loads a checkpoint from the configured checkpointer.
func (s *ChannelState) LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	s.checkpointerMu.RLock()
	checkpointer := s.checkpointer
	s.checkpointerMu.RUnlock()

	if checkpointer == nil {
		return nil, fmt.Errorf("no checkpointer configured")
	}

	return checkpointer.Load(ctx, runID)
}

// SetCheckpointer configures the checkpointer to use for state persistence.
func (s *ChannelState) SetCheckpointer(checkpointer checkpoint.Checkpointer) {
	s.checkpointerMu.Lock()
	defer s.checkpointerMu.Unlock()
	s.checkpointer = checkpointer
}

// =============================================================================
// State - Message Management
// =============================================================================

// AddMessages appends messages to the state's message history.
func (s *ChannelState) AddMessages(messages []ExecutionResult) {
	if len(messages) == 0 {
		return
	}

	ch, ok := s.GetChannel("messages")
	if !ok {
		return
	}

	values := make([]any, len(messages))
	for i := range messages {
		values[i] = messages[i]
	}

	_ = ch.Write(context.Background(), values) // Ignore error - internal initialization
}

// SetMaxMessages updates the retention limit of the "messages" channel without
// discarding existing channel configuration.
func (s *ChannelState) SetMaxMessages(maxMessages int) {
	if maxMessages < 0 {
		maxMessages = 0
	}

	// No lock needed - Set.Get() and Set.Add() handle their own locking
	ch, ok := s.channels.Get("messages")
	if !ok {
		s.channels.Add(channel.NewTopicChannel("messages", maxMessages))
		return
	}

	if topic, ok := ch.(*channel.TopicChannel); ok {
		topic.SetMaxValues(maxMessages)
		return
	}

	// Fallback: replace with a new topic channel while attempting to preserve messages.
	snapshot, err := ch.Read(context.Background())
	if err != nil {
		snapshot = nil
	}

	newTopic := channel.NewTopicChannel("messages", maxMessages)
	if values, ok := snapshot.([]any); ok && len(values) > 0 {
		_ = newTopic.Write(context.Background(), values)
	}

	s.channels.Add(newTopic)
}

// ApplyUpdates applies state updates and messages to the state manager.
func (s *ChannelState) ApplyUpdates(values map[string]any, messages []ExecutionResult) {
	ctx := context.Background()

	// No lock needed - Set methods handle their own locking
	for key, value := range values {
		ch, exists := s.channels.Get(key)
		if !exists {
			ch = channel.NewLastValueChannel(key)
			s.channels.Add(ch)
		}
		_ = ch.Write(ctx, value) // Ignore error - updates are best-effort
	}

	if len(messages) > 0 {
		if ch, ok := s.channels.Get("messages"); ok {
			vals := make([]any, len(messages))
			for i := range messages {
				vals[i] = messages[i]
			}
			_ = ch.Write(ctx, vals) // Ignore error - updates are best-effort
		}
	}
}

// Set sets a value in the state for the given key.
func (s *ChannelState) Set(key string, value any) error {
	// No lock needed - Set methods handle their own locking
	ch, exists := s.channels.Get(key)
	if !exists {
		ch = channel.NewLastValueChannel(key)
		s.channels.Add(ch)
	}
	return ch.Write(context.Background(), value)
}

// =============================================================================
// State - State Snapshots
// =============================================================================

// Snapshot returns a snapshot of all channel values.
func (s *ChannelState) Snapshot() map[string]any {
	// No lock needed - Set.SnapshotAll() handles its own locking
	values, err := s.channels.SnapshotAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

// Clone creates a deep copy of the state manager.
func (s *ChannelState) Clone() StateManager {
	// Create new state with same channel configuration
	cloned := &ChannelState{
		channels:   channel.NewSet(),
		aggregates: make(map[string]any),
	}

	// Clone checkpointer (thread-safe read)
	s.checkpointerMu.RLock()
	cloned.checkpointer = s.checkpointer
	s.checkpointerMu.RUnlock()

	// Clone all channels (Set methods handle their own locking)
	// Use VersionedChannel interface for cloning internal state
	for _, name := range s.channels.List() {
		if ch, ok := s.channels.Get(name); ok {
			// Type assert to VersionedChannel for Clone operation
			if vch, ok := ch.(channel.VersionedChannel); ok {
				cloned.channels.Add(vch.Clone())
			}
		}
	}

	// Copy aggregates (separate lock)
	s.aggregatesMu.RLock()
	maps.Copy(cloned.aggregates, s.aggregates)
	cloned.aggregateFn = s.aggregateFn
	s.aggregatesMu.RUnlock()

	return cloned
}

// =============================================================================
// ChannelState - Convenience Methods
// =============================================================================

// SnapshotAll is an alias for Snapshot for backward compatibility.
func (s *ChannelState) SnapshotAll() map[string]any {
	return s.Snapshot() // Alias
}

// ListChannels returns the names of all channels in the state.
func (s *ChannelState) ListChannels() []string {
	// No lock needed - Set.List() handles its own locking
	return s.channels.List()
}

// Ensure ChannelState implements StateManager interface
var _ StateManager = (*ChannelState)(nil)

// =============================================================================
// State Adapters
// =============================================================================

// StateReaderAdapter adapts StateManager to Reader interface.
// This provides a read-only view of the state, preventing nodes from
// directly mutating state outside of the BSP model (updates must go
// through NodeResult).
//
//nolint:revive // StateReaderAdapter is an established API name
type StateReaderAdapter struct {
	manager StateManager
}

// NewStateReaderAdapter creates a read-only view of a StateManager.
// Nodes receive this interface to prevent direct state mutations.
func NewStateReaderAdapter(manager StateManager) *StateReaderAdapter {
	return &StateReaderAdapter{manager: manager}
}

// Get retrieves a value by key.
func (sr *StateReaderAdapter) Get(key string) any {
	return sr.manager.Get(key)
}

// GetAll returns all state values.
func (sr *StateReaderAdapter) GetAll() map[string]any {
	return sr.manager.GetAll()
}

// MessagesSnapshot returns current message history.
func (sr *StateReaderAdapter) MessagesSnapshot() []ExecutionResult {
	return sr.manager.MessagesSnapshot()
}

// AggregatesSnapshot returns aggregate values.
func (sr *StateReaderAdapter) AggregatesSnapshot() map[string]any {
	return sr.manager.GetAggregatesSnapshot()
}

// Type-safe accessor methods (delegate to underlying manager)

func (sr *StateReaderAdapter) GetString(key string) (string, error) {
	return sr.manager.GetString(key)
}

func (sr *StateReaderAdapter) GetInt(key string) (int, error) {
	return sr.manager.GetInt(key)
}

func (sr *StateReaderAdapter) GetInt64(key string) (int64, error) {
	return sr.manager.GetInt64(key)
}

func (sr *StateReaderAdapter) GetFloat64(key string) (float64, error) {
	return sr.manager.GetFloat64(key)
}

func (sr *StateReaderAdapter) GetBool(key string) (bool, error) {
	return sr.manager.GetBool(key)
}

func (sr *StateReaderAdapter) GetSlice(key string) ([]any, error) {
	return sr.manager.GetSlice(key)
}

func (sr *StateReaderAdapter) GetMap(key string) (map[string]any, error) {
	return sr.manager.GetMap(key)
}

// StateWriterAdapter adapts StateManager to Writer interface.
// This extends Reader with aggregation capabilities, allowing
// nodes to contribute to global aggregators during execution.
//
//nolint:revive // StateWriterAdapter is an established API name
type StateWriterAdapter struct {
	*StateReaderAdapter
	manager StateManager
}

// NewStateWriterAdapter creates a read-write view of a StateManager.
// This is the interface that nodes receive during execution, providing
// read access to state and write access to aggregators.
func NewStateWriterAdapter(manager StateManager) *StateWriterAdapter {
	return &StateWriterAdapter{
		StateReaderAdapter: NewStateReaderAdapter(manager),
		manager:            manager,
	}
}

// Set updates a state value.
func (sw *StateWriterAdapter) Set(key string, value any) error {
	return sw.manager.Set(key, value)
}

// Aggregate performs an aggregation operation.
func (sw *StateWriterAdapter) Aggregate(name string, value any) error {
	return sw.manager.RecordAggregation(name, value)
}

// =============================================================================
// bufferedStateWriter - Internal BSP helper
// =============================================================================

// BufferedStateWriter wraps a Reader and buffers all Aggregate() calls.
// This ensures mutations are not visible within the same superstep, maintaining
// Pregel's BSP (Bulk Synchronous Parallel) semantics where all updates become
// visible only after the superstep barrier.
//
// Without buffering, aggregates would be immediately visible to other vertices
// in the same superstep, breaking the BSP model's guarantee of deterministic
// execution order independence.
//
// The buffered aggregates are flushed at the end of node execution and applied
// to the runtime's aggregator state for the next superstep.
type BufferedStateWriter struct {
	reader            Reader
	pendingAggregates map[string][]any
	mu                sync.Mutex
}

// NewBufferedStateWriter creates a new buffered state writer that wraps a reader.
// Used internally by the Pregel runtime to maintain BSP semantics.
func NewBufferedStateWriter(reader Reader) *BufferedStateWriter {
	return &BufferedStateWriter{
		reader:            reader,
		pendingAggregates: make(map[string][]any),
	}
}

// Get retrieves a value by key from the underlying reader.
func (bsw *BufferedStateWriter) Get(key string) any {
	return bsw.reader.Get(key)
}

// GetAll returns all state values from the underlying reader.
func (bsw *BufferedStateWriter) GetAll() map[string]any {
	return bsw.reader.GetAll()
}

// MessagesSnapshot returns current message history from the underlying reader.
func (bsw *BufferedStateWriter) MessagesSnapshot() []ExecutionResult {
	return bsw.reader.MessagesSnapshot()
}

// AggregatesSnapshot returns aggregate values from the underlying reader.
func (bsw *BufferedStateWriter) AggregatesSnapshot() map[string]any {
	return bsw.reader.AggregatesSnapshot()
}

// Type-safe accessor methods (delegate to underlying reader)

func (bsw *BufferedStateWriter) GetString(key string) (string, error) {
	return bsw.reader.GetString(key)
}

func (bsw *BufferedStateWriter) GetInt(key string) (int, error) {
	return bsw.reader.GetInt(key)
}

func (bsw *BufferedStateWriter) GetInt64(key string) (int64, error) {
	return bsw.reader.GetInt64(key)
}

func (bsw *BufferedStateWriter) GetFloat64(key string) (float64, error) {
	return bsw.reader.GetFloat64(key)
}

func (bsw *BufferedStateWriter) GetBool(key string) (bool, error) {
	return bsw.reader.GetBool(key)
}

func (bsw *BufferedStateWriter) GetSlice(key string) ([]any, error) {
	return bsw.reader.GetSlice(key)
}

func (bsw *BufferedStateWriter) GetMap(key string) (map[string]any, error) {
	return bsw.reader.GetMap(key)
}

// Set is not supported on BufferedStateWriter. State writes must go through NodeResult.
func (bsw *BufferedStateWriter) Set(key string, value any) error {
	// bufferedStateWriter doesn't support Set - writes go through NodeResult
	return fmt.Errorf("bufferedStateWriter.Set is not supported: state writes must go through NodeResult")
}

// Aggregate buffers an aggregate value. The value is not immediately visible;
// it becomes available in the next superstep after FlushAggregates is called.
func (bsw *BufferedStateWriter) Aggregate(name string, value any) error {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()

	if name == "" {
		return fmt.Errorf("aggregate name cannot be empty")
	}

	bsw.pendingAggregates[name] = append(bsw.pendingAggregates[name], value)
	return nil
}

// FlushAggregates returns all buffered aggregates and clears the buffer.
// Returns nil if no aggregates are buffered. Thread-safe.
func (bsw *BufferedStateWriter) FlushAggregates() map[string][]any {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()

	if len(bsw.pendingAggregates) == 0 {
		return nil
	}

	flushed := make(map[string][]any, len(bsw.pendingAggregates))
	for k, v := range bsw.pendingAggregates {
		copied := make([]any, len(v))
		copy(copied, v)
		flushed[k] = copied
	}
	bsw.pendingAggregates = make(map[string][]any)

	return flushed
}

// ResetAggregates clears all buffered aggregates without returning them.
// Thread-safe. Used for cleanup and testing.
func (bsw *BufferedStateWriter) ResetAggregates() {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()
	if len(bsw.pendingAggregates) == 0 {
		return
	}
	bsw.pendingAggregates = make(map[string][]any)
}

// =============================================================================
// Helper functions
// =============================================================================

// cloneMessages creates a deep copy of a message slice for immutability.
