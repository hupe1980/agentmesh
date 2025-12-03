package graph

import (
	"context"
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// StateKey is the interface that all state keys must implement.
// Both Key[T] and ListKey[T] satisfy this interface.
type StateKey interface {
	// Name returns the unique name of this key.
	Name() string

	// IsList returns true if this is a ListKey.
	IsList() bool

	// stateKey is a marker method to prevent external implementations.
	stateKey()
}

// Key defines a typed state channel.
// Zero value is used as default.
type Key[T any] struct {
	name string
	zero T
}

// stateKey implements StateKey.
func (Key[T]) stateKey() {}

// IsList returns false for Key.
func (Key[T]) IsList() bool { return false }

// NewKey creates a state key with a default value.
func NewKey[T any](name string, defaultValue T) Key[T] {
	return Key[T]{name: name, zero: defaultValue}
}

// Name returns the key name.
func (k Key[T]) Name() string {
	return k.name
}

// Default returns the default value.
func (k Key[T]) Default() T {
	return k.zero
}

// ListKey defines a list-based state channel with aggregation.
type ListKey[T any] struct {
	name string
}

// stateKey implements StateKey.
func (ListKey[T]) stateKey() {}

// IsList returns true for ListKey.
func (ListKey[T]) IsList() bool { return true }

// NewListKey creates a list state key.
func NewListKey[T any](name string) ListKey[T] {
	return ListKey[T]{name: name}
}

// Name returns the key name.
func (k ListKey[T]) Name() string {
	return k.name
}

// SliceValue is an interface for values that can be iterated as slices.
// Implement this interface to provide slice semantics without reflection,
// improving performance and type safety.
//
// Example:
//
//	type Messages []message.Message
//
//	func (m Messages) SliceIter(yield func(any) bool) {
//	    for _, msg := range m {
//	        if !yield(msg) { return }
//	    }
//	}
type SliceValue interface {
	// SliceIter iterates over the slice, calling yield for each element.
	SliceIter(yield func(any) bool)
	// Merge appends another SliceValue and returns the result.
	// Returns nil if types are incompatible.
	Merge(other SliceValue) SliceValue
}

// SliceOf is a generic helper that wraps any slice type to implement SliceValue.
// This provides a convenient way to iterate strongly-typed slices without reflection.
//
// Example:
//
//	messages := []message.Message{msg1, msg2}
//	SliceOf(messages).SliceIter(func(item any) bool { ... })
type SliceOf[T any] []T

// SliceIter iterates over the slice, calling yield for each element.
func (s SliceOf[T]) SliceIter(yield func(any) bool) {
	for _, v := range s {
		if !yield(v) {
			return
		}
	}
}

// Merge appends another SliceValue of the same type and returns the result.
func (s SliceOf[T]) Merge(other SliceValue) SliceValue {
	if o, ok := other.(SliceOf[T]); ok {
		return append(s, o...)
	}
	return nil
}

// View provides read access to state.
type View interface {
	// GetValue returns the raw value for a key name.
	GetValue(name string) (any, bool)

	// ManagedValues returns the managed values registry, or nil if not configured.
	ManagedValues() *managedValueRegistry
}

// Get returns the typed value for a key from the view.
func Get[T any](view View, key Key[T]) T {
	if v, ok := view.GetValue(key.name); ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}
	return key.zero
}

// GetList returns the typed list for a list key from the view.
// Handles both []T and SliceOf[T] storage formats.
func GetList[T any](view View, key ListKey[T]) []T {
	if v, ok := view.GetValue(key.name); ok {
		// Handle SliceOf[T] (used by Append/AppendValue for zero-reflection)
		if sliceOf, ok := v.(SliceOf[T]); ok {
			return []T(sliceOf)
		}
		// Handle plain []T (legacy or external sources)
		if typed, ok := v.([]T); ok {
			return typed
		}
	}
	return nil
}

// Updates is a map of state changes.
type Updates map[string]any

// Store interface for state persistence.
type Store interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any) error
	Delete(ctx context.Context, key string) error
}

// memoryStore is the default in-memory store.
type memoryStore struct {
	data map[string]any
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		data: make(map[string]any),
	}
}

func (s *memoryStore) Get(ctx context.Context, key string) (any, error) {
	return s.data[key], nil
}

func (s *memoryStore) Set(ctx context.Context, key string, value any) error {
	s.data[key] = value
	return nil
}

func (s *memoryStore) Delete(ctx context.Context, key string) error {
	delete(s.data, key)
	return nil
}

// stateView implements View for reading state.
type stateView struct {
	data    map[string]any
	managed *managedValueRegistry
}

func (v *stateView) GetValue(name string) (any, bool) {
	val, ok := v.data[name]
	return val, ok
}

func (v *stateView) ManagedValues() *managedValueRegistry {
	return v.managed
}

// -----------------------------------------------------------------------------
// BSP State Manager - Optimized Bulk-Synchronous Parallel semantics
// -----------------------------------------------------------------------------

// BSPState manages state with proper BSP semantics:
// - All reads within a superstep see the same snapshot (from previous superstep)
// - All writes are buffered and only become visible after barrier commit
// - This ensures deterministic parallel execution regardless of scheduling order
//
// Optimizations:
// - Copy-on-write: readSnapshot only recreated when writes occur
// - Version tracking: avoid unnecessary snapshot copies
// - Type switches: avoid reflection for common slice types
// - Atomic version checking: skip locking in ReadView when possible
type BSPState struct {
	mu sync.RWMutex

	// readSnapshot is the immutable state visible to all nodes in current superstep.
	// Created at superstep start from committed state.
	// Protected by mu for writes, but can be read atomically via cachedView.
	readSnapshot map[string]any

	// writeBuffer accumulates all writes during current superstep.
	// Writes are merged (lists appended, scalars overwritten).
	// Committed to readSnapshot at superstep barrier.
	writeBuffer map[string]any

	// committed is the authoritative state after all barriers.
	// Used for checkpointing and final output.
	committed map[string]any
	// ownsCommitted indicates whether committed is safe to mutate without cloning.
	ownsCommitted bool

	// version tracks state changes for copy-on-write optimization.
	// Incremented on each barrier commit. Accessed atomically.
	version atomic.Uint64

	// snapshotVersion tracks which version the current readSnapshot is from.
	// If equal to version, readSnapshot is still valid (no copy needed).
	snapshotVersion atomic.Uint64

	// cachedView is an atomically-swapped cached view for lock-free reads.
	// Updated when readSnapshot changes.
	cachedView atomic.Pointer[stateView]

	// managedValues holds ephemeral runtime state (not checkpointed).
	managedValues *managedValueRegistry

	// pendingWrites captures provenance information for two-phase commit checkpoints.
	// Each entry records which node wrote to which channel along with the raw value.
	pendingWrites []checkpoint.PendingWrite
}

// NewBSPState creates a new BSP-compliant state manager.
func NewBSPState(initial map[string]any) *BSPState {
	var committed map[string]any
	ownedCommitted := false
	if initial == nil {
		committed = make(map[string]any)
		ownedCommitted = true
	} else {
		committed = initial
	}

	// Initial read snapshot shares the committed map (copy-on-write)
	// This is safe because we'll clone on first mutation
	state := &BSPState{
		readSnapshot:  committed, // Share initially (CoW)
		writeBuffer:   make(map[string]any),
		committed:     committed,
		ownsCommitted: ownedCommitted,
	}
	// version and snapshotVersion start at 0 (zero value)
	// Set initial cached view
	state.cachedView.Store(&stateView{data: committed})
	return state
}

// setManagedValues attaches a managed values registry to the state (internal).
func (s *BSPState) setManagedValues(registry *managedValueRegistry) {
	s.managedValues = registry
	// Update cached view to include managed values
	s.mu.RLock()
	s.cachedView.Store(&stateView{data: s.readSnapshot, managed: registry})
	s.mu.RUnlock()
}

// ReadView returns a View that reads from the current superstep's snapshot.
// This view is safe for concurrent reads - it reads from immutable snapshot.
// Optimized: uses atomic pointer to cached view, avoiding lock acquisition
// when snapshot hasn't changed.
func (s *BSPState) ReadView() View {
	// Fast path: return cached view without locking
	// The cached view is atomically updated whenever readSnapshot changes
	if cached := s.cachedView.Load(); cached != nil {
		return cached
	}

	// Fallback: create view under lock (should rarely happen)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &stateView{data: s.readSnapshot, managed: s.managedValues}
}

// Write buffers a state update for the current superstep.
// The update will only be visible after CommitBarrier is called.
// Thread-safe for concurrent writes from parallel nodes.
func (s *BSPState) Write(nodeName string, updates Updates) {
	if len(updates) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, value := range updates {
		s.mergeWrite(key, value)
		s.pendingWrites = append(s.pendingWrites, checkpoint.PendingWrite{
			NodeName:  nodeName,
			Channel:   key,
			Value:     value,
			Timestamp: time.Now(),
		})
	}
}

// mergeWrite merges a value into the write buffer.
// For slices: appends to existing slice in write buffer (or creates new)
// For scalars: overwrites (last writer wins within superstep)
// Optimized: uses type switches for common types to avoid reflection.
// Must be called while holding s.mu lock.
func (s *BSPState) mergeWrite(key string, value any) {
	// Fast path: check common non-slice types without reflection
	switch value.(type) {
	case string, int, int64, float64, bool, nil:
		s.writeBuffer[key] = value
		return
	}

	// Check if it's a slice using type switches for common types
	existing, exists := s.writeBuffer[key]
	if !exists {
		s.writeBuffer[key] = value
		return
	}

	// Try to merge slices using type switches (avoid reflection)
	merged := mergeSlices(existing, value)
	s.writeBuffer[key] = merged
}

// mergeSlices merges two values if they are slices with compatible element types.
// Uses type switches for common types to avoid reflection.
// Falls back to reflection for unknown slice types.
// Handles SliceOf[T] and []T interoperability.
func mergeSlices(existing, value any) any {
	// Fast path: both implement SliceValue (covers all SliceOf[T] types)
	if ev, ok := existing.(SliceValue); ok {
		if vv, ok := value.(SliceValue); ok {
			if merged := ev.Merge(vv); merged != nil {
				return merged
			}
		}
	}

	// Try common slice types first (fast path, no reflection)
	switch v := value.(type) {
	case []string:
		if e, ok := existing.([]string); ok {
			return append(e, v...)
		}
	case []int:
		if e, ok := existing.([]int); ok {
			return append(e, v...)
		}
	case []any:
		if e, ok := existing.([]any); ok {
			return append(e, v...)
		}
	case []byte:
		if e, ok := existing.([]byte); ok {
			return append(e, v...)
		}
	}

	// Reflection fallback (rarely hit)
	return mergeSlicesReflection(existing, value)
}

// mergeSlicesReflection merges slices using reflection.
// This is a fallback for slice types not covered by type switches.
func mergeSlicesReflection(existing, value any) any {
	existingVal := reflect.ValueOf(existing)
	newVal := reflect.ValueOf(value)

	if existingVal.Kind() != reflect.Slice || newVal.Kind() != reflect.Slice {
		return value
	}

	// Check if element types are compatible
	if existingVal.Type().Elem() != newVal.Type().Elem() {
		return value
	}

	// Merge slices
	merged := reflect.MakeSlice(existingVal.Type(), 0, existingVal.Len()+newVal.Len())
	merged = reflect.AppendSlice(merged, existingVal)
	merged = reflect.AppendSlice(merged, newVal)

	return merged.Interface()
}

// CommitBarrier commits all buffered writes and creates a new read snapshot.
// This is called at the end of each superstep (barrier synchronization point).
// Optimized: only creates new snapshot if there were writes.
func (s *BSPState) CommitBarrier() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Fast path: no writes, nothing to commit
	if len(s.writeBuffer) == 0 {
		return
	}

	// Merge write buffer into committed state
	for key, value := range s.writeBuffer {
		s.mergeIntoCommitted(key, value)
	}

	// Clear write buffer for next superstep (reuse map if small)
	if len(s.writeBuffer) > 16 {
		s.writeBuffer = make(map[string]any)
	} else {
		clear(s.writeBuffer)
	}

	// Clear pending write metadata now that updates are committed
	if len(s.pendingWrites) > 0 {
		if cap(s.pendingWrites) > 32 {
			s.pendingWrites = nil
		} else {
			s.pendingWrites = s.pendingWrites[:0]
		}
	}

	// Increment version and create new read snapshot
	newVersion := s.version.Add(1)
	s.readSnapshot = make(map[string]any, len(s.committed))
	maps.Copy(s.readSnapshot, s.committed)
	s.snapshotVersion.Store(newVersion)
	// Update cached view atomically for lock-free reads
	s.cachedView.Store(&stateView{data: s.readSnapshot, managed: s.managedValues})
}

// mergeIntoCommitted merges a value from write buffer into committed state.
// Optimized: uses type switches for common slice types.
// Must be called while holding s.mu lock.
func (s *BSPState) mergeIntoCommitted(key string, value any) {
	s.ensureCommittedOwnership()

	// Fast path for common scalar types
	switch value.(type) {
	case string, int, int64, float64, bool, nil:
		s.committed[key] = value
		return
	}

	existing, exists := s.committed[key]
	if !exists {
		s.committed[key] = value
		return
	}

	// Merge slices
	s.committed[key] = mergeSlices(existing, value)
}

// Snapshot returns a copy of the committed state.
// Used for checkpointing and final result extraction.
func (s *BSPState) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make(map[string]any, len(s.committed))
	maps.Copy(snapshot, s.committed)
	return snapshot
}

// GetCommitted returns a value from committed state.
// Used for extracting final output after execution completes.
func (s *BSPState) GetCommitted(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.committed[key]
	return val, ok
}

// PendingWrites returns write buffer contents as checkpoint.PendingWrite slice.
// Used for two-phase commit checkpointing - captures writes before barrier commit.
func (s *BSPState) PendingWrites() []checkpoint.PendingWrite {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.pendingWrites) == 0 {
		return nil
	}

	pending := make([]checkpoint.PendingWrite, len(s.pendingWrites))
	copy(pending, s.pendingWrites)
	return pending
}

// HasPendingWrites returns true if there are uncommitted writes in the buffer.
func (s *BSPState) HasPendingWrites() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.pendingWrites) > 0
}

// ApplyPendingWrites applies externally provided pending writes to committed state.
// Used when restoring from a checkpoint with Committed=false.
// The writes are applied directly to committed state (since the checkpoint
// was saved after node execution but before barrier commit).
func (s *BSPState) ApplyPendingWrites(pending []checkpoint.PendingWrite) {
	if len(pending) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureCommittedOwnership()

	// Apply pending writes to committed state
	for _, write := range pending {
		s.mergeIntoCommitted(write.Channel, write.Value)
	}

	// Update read snapshot to reflect applied writes
	newVersion := s.version.Add(1)
	s.readSnapshot = make(map[string]any, len(s.committed))
	maps.Copy(s.readSnapshot, s.committed)
	s.snapshotVersion.Store(newVersion)
	// Update cached view atomically for lock-free reads
	s.cachedView.Store(&stateView{data: s.readSnapshot, managed: s.managedValues})
}

// ensureCommittedOwnership clones the committed state before mutation when needed.
func (s *BSPState) ensureCommittedOwnership() {
	if s.ownsCommitted {
		return
	}
	cloned := make(map[string]any, len(s.committed))
	maps.Copy(cloned, s.committed)
	s.committed = cloned
	s.ownsCommitted = true
}
