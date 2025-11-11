package graph

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// =============================================================================
// State Interfaces
// =============================================================================

// StateReader provides read-only access to graph state for deterministic node execution.
// Nodes receive this interface to prevent direct state mutations, ensuring all updates
// go through the NodeResult return value for atomic application between supersteps.
type StateReader interface {
	// Get retrieves the current value from a named channel.
	Get(key string) any

	// GetAll returns a snapshot of all channel values.
	GetAll() map[string]any

	// EventsSnapshot returns message events from the "messages" channel.
	EventsSnapshot() []Event

	// AggregatesSnapshot returns a copy of global aggregates from the previous superstep.
	AggregatesSnapshot() map[string]any
}

// StateWriter extends StateReader with mutation capabilities for aggregators.
type StateWriter interface {
	StateReader

	// Aggregate contributes a value to a named aggregator for the current superstep.
	Aggregate(name string, value any) error
}

// =============================================================================
// StateManager Interface
// =============================================================================

// StateManager owns all state concerns: channels, checkpoints, and aggregates.
// This provides a clean separation of state management from execution logic.
//
// Responsibilities:
//   - Channel-based state management (TopicChannel, LastValueChannel, BinaryOpChannel)
//   - Checkpoint persistence and restoration
//   - Aggregate value management (cross-node reductions)
//   - Thread-safe state access and mutations
//
// Design Goals:
//   - Single source of truth for all graph state
//   - Clean interface for state reads and writes
//   - Pluggable checkpoint backends
//   - No execution logic (pure state management)
type StateManager interface {
	// State access
	Get(key string) any
	GetAll() map[string]any
	EventsSnapshot() []Event
	AggregatesSnapshot() map[string]any

	// Channel management
	AddChannel(ch channel.Channel)
	GetChannel(name string) (channel.Channel, bool)
	UpdateChannel(ctx context.Context, name string, value any) error
	UpdateChannels(ctx context.Context, updates map[string]any) error

	// Aggregate management
	GetAggregate(name string) any
	GetAggregatesSnapshot() map[string]any
	SetAggregates(aggregates map[string]any)
	SetAggregateFn(fn func(string, any) error)
	RecordAggregation(name string, value any) error

	// Checkpoint management
	SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error
	LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
	SetCheckpointer(checkpointer checkpoint.Checkpointer)

	// Message management
	AddMessages(messages []Event)
	ApplyUpdates(values map[string]any, messages []Event)
	Set(key string, value any) error

	// Version management
	Version() uint64

	// State snapshots
	Snapshot() map[string]any
	Clone() StateManager
}

// =============================================================================
// State - Primary StateManager Implementation
// =============================================================================

// State is the primary implementation of the StateManager interface.
// It manages data flow through typed channels with thread-safe access.
//
// State serves as both:
// - The user-facing API for building graphs (via NewState, Graph.State)
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
// State manages channel-based state for graph execution.
type State struct {
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

// NewStateManager creates a new StateManager with the default State implementation.
// It automatically creates a standard "messages" channel (Topic with maxMessages limit).
// This is the recommended way to create a state manager for graph execution.
func NewStateManager(maxMessages int) StateManager {
	channels := channel.NewSet()
	channels.Add(channel.NewTopicChannel("messages", maxMessages))
	return &State{
		channels:   channels,
		aggregates: make(map[string]any),
	}
}

// NewState creates a new channel-based graph state.
// This is kept for internal use and direct *State access.
// For normal usage, prefer NewStateManager() which returns the StateManager interface.
func NewState(maxMessages int) *State {
	channels := channel.NewSet()
	channels.Add(channel.NewTopicChannel("messages", maxMessages))
	return &State{
		channels:   channels,
		aggregates: make(map[string]any),
	}
}

// =============================================================================
// State - State Read Methods
// =============================================================================

// Get retrieves a value from the state by key.
func (s *State) Get(key string) any {
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
func (s *State) GetAll() map[string]any {
	// No lock needed - Set.ReadAll() handles its own locking
	values, err := s.channels.ReadAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

// EventsSnapshot returns a copy of current message events with metadata.
func (s *State) EventsSnapshot() []Event {
	ch, ok := s.GetChannel("messages")
	if !ok {
		return nil
	}

	val, err := ch.Read(context.Background())
	if err != nil || val == nil {
		return nil
	}

	// Convert []any to []Event
	values, ok := val.([]any)
	if !ok || len(values) == 0 {
		return nil
	}

	events := make([]Event, 0, len(values))
	for _, v := range values {
		if evt, ok := v.(Event); ok {
			events = append(events, evt)
		}
	}
	return events
}

// =============================================================================
// State - Channel Management
// =============================================================================

// AddChannel registers a new channel in the state.
func (s *State) AddChannel(ch channel.Channel) {
	// No lock needed - Set.Add() handles its own locking
	s.channels.Add(ch)
}

// GetChannel retrieves a channel by name.
func (s *State) GetChannel(name string) (channel.Channel, bool) {
	// No lock needed - Set.Get() handles its own locking
	return s.channels.Get(name)
}

// UpdateChannel writes a value to a specific channel.
func (s *State) UpdateChannel(ctx context.Context, name string, value any) error {
	// No lock needed - Set.Get() and Channel.Write() handle their own locking
	ch, ok := s.channels.Get(name)
	if !ok {
		return nil // Silently ignore unknown channels
	}
	return ch.Write(ctx, value)
}

// UpdateChannels batch-updates multiple channels.
func (s *State) UpdateChannels(ctx context.Context, updates map[string]any) error {
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
func (s *State) GetAggregate(name string) any {
	s.aggregatesMu.RLock()
	defer s.aggregatesMu.RUnlock()
	return s.aggregates[name]
}

// GetAggregatesSnapshot returns a read-only snapshot of all aggregates.
func (s *State) GetAggregatesSnapshot() map[string]any {
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
func (s *State) AggregatesSnapshot() map[string]any {
	return s.GetAggregatesSnapshot()
}

// =============================================================================
// State - Version Management
// =============================================================================

// incrementVersion atomically increments the state version counter.
// Called after any state mutation to track state evolution for checkpoint integrity.
func (s *State) incrementVersion() {
	s.versionMu.Lock()
	s.version++
	s.versionMu.Unlock()
}

// Version returns the current state version.
// This monotonic counter increases with every state mutation.
func (s *State) Version() uint64 {
	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	return s.version
}

// setVersion explicitly sets the version (used during checkpoint restore).
func (s *State) setVersion(v uint64) {
	s.versionMu.Lock()
	s.version = v
	s.versionMu.Unlock()
}

// =============================================================================
// State - Aggregate Management (continued)
// =============================================================================

// SetAggregates replaces all aggregates with the provided map.
func (s *State) SetAggregates(aggregates map[string]any) {
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
func (s *State) SetAggregateFn(fn func(string, any) error) {
	s.aggregatesMu.Lock()
	defer s.aggregatesMu.Unlock()
	s.aggregateFn = fn
}

// RecordAggregation records a value for aggregation using the configured aggregation function.
func (s *State) RecordAggregation(name string, value any) error {
	s.aggregatesMu.RLock()
	fn := s.aggregateFn
	s.aggregatesMu.RUnlock()

	if fn == nil {
		return ErrAggregatorsNotConfigured
	}
	return fn(name, value)
}

// Aggregate is an alias for RecordAggregation for backward compatibility.
func (s *State) Aggregate(name string, value any) error {
	return s.RecordAggregation(name, value)
}

// =============================================================================
// State - Checkpoint Management
// =============================================================================

// SaveCheckpoint saves the current state to the configured checkpointer.
func (s *State) SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error {
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
func (s *State) LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	s.checkpointerMu.RLock()
	checkpointer := s.checkpointer
	s.checkpointerMu.RUnlock()

	if checkpointer == nil {
		return nil, fmt.Errorf("no checkpointer configured")
	}

	return checkpointer.Load(ctx, runID)
}

// SetCheckpointer configures the checkpointer to use for state persistence.
func (s *State) SetCheckpointer(checkpointer checkpoint.Checkpointer) {
	s.checkpointerMu.Lock()
	defer s.checkpointerMu.Unlock()
	s.checkpointer = checkpointer
}

// =============================================================================
// State - Message Management
// =============================================================================

// AddMessages appends messages to the state's message history.
func (s *State) AddMessages(messages []Event) {
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
func (s *State) SetMaxMessages(maxMessages int) {
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
func (s *State) ApplyUpdates(values map[string]any, messages []Event) {
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
func (s *State) Set(key string, value any) error {
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
func (s *State) Snapshot() map[string]any {
	// No lock needed - Set.SnapshotAll() handles its own locking
	values, err := s.channels.SnapshotAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

// Clone creates a deep copy of the state manager.
func (s *State) Clone() StateManager {
	// Create new state with same channel configuration
	cloned := &State{
		channels:   channel.NewSet(),
		aggregates: make(map[string]any),
	}

	// Clone checkpointer (thread-safe read)
	s.checkpointerMu.RLock()
	cloned.checkpointer = s.checkpointer
	s.checkpointerMu.RUnlock()

	// Clone all channels (Set methods handle their own locking)
	for _, name := range s.channels.List() {
		if ch, ok := s.channels.Get(name); ok {
			cloned.channels.Add(ch.Clone())
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
// State - Convenience Methods
// =============================================================================

// SnapshotAll is an alias for Snapshot for backward compatibility.
func (s *State) SnapshotAll() map[string]any {
	return s.Snapshot() // Alias
}

// ListChannels returns the names of all channels in the state.
func (s *State) ListChannels() []string {
	// No lock needed - Set.List() handles its own locking
	return s.channels.List()
}

// Ensure State implements StateManager interface
var _ StateManager = (*State)(nil)

// =============================================================================
// State Adapters
// =============================================================================

// StateReaderAdapter adapts StateManager to StateReader interface.
// This provides a read-only view of the state, preventing nodes from
// directly mutating state outside of the BSP model (updates must go
// through NodeResult).
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

// EventsSnapshot returns current message events.
func (sr *StateReaderAdapter) EventsSnapshot() []Event {
	return sr.manager.EventsSnapshot()
}

// AggregatesSnapshot returns aggregate values.
func (sr *StateReaderAdapter) AggregatesSnapshot() map[string]any {
	return sr.manager.GetAggregatesSnapshot()
}

// StateWriterAdapter adapts StateManager to StateWriter interface.
// This extends StateReader with aggregation capabilities, allowing
// nodes to contribute to global aggregators during execution.
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

// Aggregate performs an aggregation operation.
func (sw *StateWriterAdapter) Aggregate(name string, value any) error {
	return sw.manager.RecordAggregation(name, value)
}

// =============================================================================
// bufferedStateWriter - Internal BSP helper
// =============================================================================

// bufferedStateWriter wraps a StateReader and buffers all Aggregate() calls.
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
type bufferedStateWriter struct {
	reader            StateReader
	pendingAggregates map[string][]any
	mu                sync.Mutex
}

func newBufferedStateWriter(reader StateReader) *bufferedStateWriter {
	return &bufferedStateWriter{
		reader:            reader,
		pendingAggregates: make(map[string][]any),
	}
}

func (bsw *bufferedStateWriter) Get(key string) any {
	return bsw.reader.Get(key)
}

func (bsw *bufferedStateWriter) GetAll() map[string]any {
	return bsw.reader.GetAll()
}

func (bsw *bufferedStateWriter) EventsSnapshot() []Event {
	return bsw.reader.EventsSnapshot()
}

func (bsw *bufferedStateWriter) AggregatesSnapshot() map[string]any {
	return bsw.reader.AggregatesSnapshot()
}

func (bsw *bufferedStateWriter) Aggregate(name string, value any) error {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()

	if name == "" {
		return fmt.Errorf("aggregate name cannot be empty")
	}

	bsw.pendingAggregates[name] = append(bsw.pendingAggregates[name], value)
	return nil
}

func (bsw *bufferedStateWriter) flushAggregates() map[string][]any {
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

func (bsw *bufferedStateWriter) resetAggregates() {
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
