package state

import (
	"context"
	"maps"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Internal Components for StateManager Implementation
//
// This file contains internal components that implement Single Responsibility Principle
// by breaking down the monolithic ChannelState into focused, composable parts:
//
// 1. ChannelStore - Manages channel registration and value updates
// 2. AggregateStore - Handles aggregate value accumulation and snapshots
// 3. CheckpointCoordinator - Manages checkpoint persistence operations
// 4. VersionTracker - Tracks state version for change detection
//
// These components are internal implementation details and are not exposed in the public API.
// The ChannelState struct composes these components and delegates to them.

// =============================================================================
// ChannelStore - Channel Management Component
// =============================================================================

// channelStore manages channel registration, lookup, and updates.
// It handles all channel-related operations with thread-safe access.
//
// Responsibilities:
// - Register and retrieve channels by name
// - Update individual channel values
// - Batch update multiple channels atomically
// - Maintain channel set for iteration
//
// Performance Optimization:
// - Implements copy-on-write semantics for snapshots
// - Caches snapshot until any channel is modified
// - Version tracking for efficient cache invalidation
//
// Thread Safety: Uses the channel.Set's internal locking plus snapshot cache protection.
type channelStore struct {
	channels *channel.Set

	// Copy-on-write snapshot cache
	mu             sync.RWMutex
	cachedSnapshot map[string]any
	cachedVersion  int64
	currentVersion int64
}

// newChannelStore creates a new channel store.
func newChannelStore() *channelStore {
	return &channelStore{
		channels:       channel.NewSet(),
		currentVersion: 0,
		cachedVersion:  -1, // Ensures first snapshot creates cache
	}
}

// addChannel registers a new channel in the store.
func (cs *channelStore) addChannel(ch channel.Channel) {
	cs.channels.Add(ch)
}

// getChannel retrieves a channel by name.
func (cs *channelStore) getChannel(name string) (channel.Channel, bool) {
	return cs.channels.Get(name)
}

// get retrieves the current value of a channel.
func (cs *channelStore) get(key string) any {
	ch, ok := cs.channels.Get(key)
	if !ok {
		return nil
	}
	val, err := ch.Read(context.Background())
	if err != nil {
		return nil
	}
	return val
}

// getAll returns all state values as a map.
func (cs *channelStore) getAll() map[string]any {
	values, err := cs.channels.ReadAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

// updateChannel writes a value to a specific channel.
func (cs *channelStore) updateChannel(ctx context.Context, name string, value any) error {
	ch, ok := cs.channels.Get(name)
	if !ok {
		return nil // Skip unknown channels
	}
	err := ch.Write(ctx, value)
	if err == nil {
		// Invalidate snapshot cache on successful write
		cs.invalidateCache()
	}
	return err
}

// updateChannels batch-updates multiple channels atomically.
func (cs *channelStore) updateChannels(ctx context.Context, updates map[string]any) error {
	err := cs.channels.WriteAll(ctx, updates)
	if err == nil {
		// Invalidate snapshot cache on successful write
		cs.invalidateCache()
	}
	return err
}

// snapshot returns a complete snapshot of all channel values.
// Implements copy-on-write semantics - returns cached snapshot if state hasn't changed.
func (cs *channelStore) snapshot() map[string]any {
	// Fast path: check if cache is valid (read lock)
	cs.mu.RLock()
	if cs.cachedVersion == cs.currentVersion && cs.cachedSnapshot != nil {
		snapshot := cs.cachedSnapshot
		cs.mu.RUnlock()
		return snapshot
	}
	cs.mu.RUnlock()

	// Slow path: create new snapshot (write lock to update cache)
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have updated)
	if cs.cachedVersion == cs.currentVersion && cs.cachedSnapshot != nil {
		return cs.cachedSnapshot
	}

	// Create snapshot from all channels
	values, err := cs.channels.SnapshotAll(context.Background())
	if err != nil {
		return nil
	}

	// Cache the snapshot
	cs.cachedSnapshot = values
	cs.cachedVersion = cs.currentVersion

	return values
}

// invalidateCache increments version and clears cached snapshot.
// Must be called after any write operation.
func (cs *channelStore) invalidateCache() {
	cs.mu.Lock()
	cs.currentVersion++
	cs.cachedSnapshot = nil
	cs.mu.Unlock()
}

// list returns the names of all channels.
func (cs *channelStore) list() []string {
	return cs.channels.List()
}

// clone creates an independent copy of the channel store.
func (cs *channelStore) clone() *channelStore {
	newStore := newChannelStore()
	// Copy all channels
	for _, name := range cs.channels.List() {
		if ch, ok := cs.channels.Get(name); ok {
			newStore.addChannel(ch)
		}
	}
	// Don't copy cache - let clone build its own
	return newStore
}

// =============================================================================
// AggregateStore - Aggregate Management Component
// =============================================================================

// aggregateStore manages aggregate values for cross-node reductions.
// Provides thread-safe storage and snapshot capabilities.
//
// Responsibilities:
// - Store and retrieve aggregate values
// - Execute aggregate functions
// - Create immutable snapshots
// - Support aggregate replacement
//
// Thread Safety: Protected by sync.RWMutex for concurrent read/write.
type aggregateStore struct {
	aggregates  map[string]any
	aggregateFn func(string, any) error
	mu          sync.RWMutex
}

// newAggregateStore creates a new aggregate store.
func newAggregateStore() *aggregateStore {
	return &aggregateStore{
		aggregates: make(map[string]any),
	}
}

// getAggregate retrieves the current value of a named aggregate.
func (as *aggregateStore) getAggregate(name string) any {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.aggregates[name]
}

// getSnapshot returns a read-only snapshot of all aggregates.
func (as *aggregateStore) getSnapshot() map[string]any {
	as.mu.RLock()
	defer as.mu.RUnlock()
	snapshot := make(map[string]any, len(as.aggregates))
	maps.Copy(snapshot, as.aggregates)
	return snapshot
}

// setAggregates replaces all aggregates with the provided map.
func (as *aggregateStore) setAggregates(aggregates map[string]any) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.aggregates = aggregates
}

// setAggregateFn configures the function used to combine aggregate values.
func (as *aggregateStore) setAggregateFn(fn func(string, any) error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.aggregateFn = fn
}

// recordAggregation records a value for aggregation.
func (as *aggregateStore) recordAggregation(name string, value any) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.aggregateFn != nil {
		return as.aggregateFn(name, value)
	}
	return nil
}

// clone creates an independent copy of the aggregate store.
func (as *aggregateStore) clone() *aggregateStore {
	as.mu.RLock()
	defer as.mu.RUnlock()

	aggregatesCopy := make(map[string]any, len(as.aggregates))
	maps.Copy(aggregatesCopy, as.aggregates)

	return &aggregateStore{
		aggregates:  aggregatesCopy,
		aggregateFn: as.aggregateFn,
	}
}

// =============================================================================
// CheckpointCoordinator - Checkpoint Management Component
// =============================================================================

// checkpointCoordinator manages checkpoint persistence operations.
// Handles save/load operations with pluggable backend support.
//
// Responsibilities:
// - Configure checkpoint backend
// - Save state snapshots to backend
// - Load state from checkpoints
// - Coordinate with ChannelStore and AggregateStore
//
// Thread Safety: Protected by sync.RWMutex for checkpointer access.
type checkpointCoordinator struct {
	checkpointer checkpoint.Checkpointer
	mu           sync.RWMutex
}

// newCheckpointCoordinator creates a new checkpoint coordinator.
func newCheckpointCoordinator() *checkpointCoordinator {
	return &checkpointCoordinator{}
}

// setCheckpointer configures the checkpoint backend.
func (cc *checkpointCoordinator) setCheckpointer(cp checkpoint.Checkpointer) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.checkpointer = cp
}

// saveCheckpoint persists current state to configured backend.
func (cc *checkpointCoordinator) saveCheckpoint(
	ctx context.Context,
	runID string,
	superstep int64,
	metadata map[string]any,
	channelSnapshot map[string]any,
	aggregateSnapshot map[string]any,
	version uint64,
) error {
	cc.mu.RLock()
	cp := cc.checkpointer
	cc.mu.RUnlock()

	if cp == nil {
		return nil // No checkpointer configured
	}

	// Merge aggregates into metadata for checkpoint storage
	checkpointMetadata := make(map[string]any, len(metadata)+1)
	maps.Copy(checkpointMetadata, metadata)
	checkpointMetadata["aggregates"] = aggregateSnapshot

	return cp.Save(ctx, &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: superstep,
		State:     channelSnapshot,
		Metadata:  checkpointMetadata,
		Version:   version,
	})
}

// loadCheckpoint restores state from a previous checkpoint.
func (cc *checkpointCoordinator) loadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	cc.mu.RLock()
	cp := cc.checkpointer
	cc.mu.RUnlock()

	if cp == nil {
		return nil, nil // No checkpointer configured
	}

	return cp.Load(ctx, runID)
}

// =============================================================================
// VersionTracker - State Version Management Component
// =============================================================================

// versionTracker manages the monotonic version counter for state changes.
// Used for checkpoint integrity and change detection.
//
// Responsibilities:
// - Maintain monotonic version counter
// - Increment version on state mutations
// - Provide thread-safe version access
//
// Thread Safety: Protected by sync.Mutex for atomic increment.
type versionTracker struct {
	version uint64
	mu      sync.Mutex
}

// newVersionTracker creates a new version tracker starting at version 0.
func newVersionTracker() *versionTracker {
	return &versionTracker{}
}

// get returns the current version.
func (vt *versionTracker) get() uint64 {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	return vt.version
}

// increment increments the version counter and returns the new version.
func (vt *versionTracker) increment() uint64 {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.version++
	return vt.version
}

// set sets the version to a specific value (used during checkpoint restore).
func (vt *versionTracker) set(version uint64) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.version = version
}
