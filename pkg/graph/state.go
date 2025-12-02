package graph

import (
	"context"
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
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
	SliceIter(yield func(any) bool)
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
}

// NewBSPState creates a new BSP-compliant state manager.
func NewBSPState(initial map[string]any) *BSPState {
	committed := make(map[string]any)
	if initial != nil {
		maps.Copy(committed, initial)
	}

	// Initial read snapshot shares the committed map (copy-on-write)
	// This is safe because we'll create a new map on first write
	state := &BSPState{
		readSnapshot: committed, // Share initially (CoW)
		writeBuffer:  make(map[string]any),
		committed:    committed,
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
func (s *BSPState) Write(updates Updates) {
	if len(updates) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, value := range updates {
		s.mergeWrite(key, value)
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

	// Fallback to reflection for other slice types (including SliceOf[T])
	existingVal := reflect.ValueOf(existing)
	newVal := reflect.ValueOf(value)

	if existingVal.Kind() != reflect.Slice || newVal.Kind() != reflect.Slice {
		return value
	}

	// Check if element types are compatible (handles SliceOf[T] + []T)
	if existingVal.Type().Elem() != newVal.Type().Elem() {
		return value
	}

	// Merge by iterating (works across SliceOf[T] and []T)
	merged := reflect.MakeSlice(existingVal.Type(), 0, existingVal.Len()+newVal.Len())
	merged = reflect.AppendSlice(merged, existingVal)
	for i := 0; i < newVal.Len(); i++ {
		merged = reflect.Append(merged, newVal.Index(i))
	}
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
func (s *BSPState) PendingWrites() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pending := make(map[string]any, len(s.writeBuffer))
	maps.Copy(pending, s.writeBuffer)
	return pending
}

// HasPendingWrites returns true if there are uncommitted writes in the buffer.
func (s *BSPState) HasPendingWrites() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.writeBuffer) > 0
}

// ApplyPendingWrites applies externally provided pending writes to committed state.
// Used when restoring from a checkpoint with Committed=false.
// The writes are applied directly to committed state (since the checkpoint
// was saved after node execution but before barrier commit).
func (s *BSPState) ApplyPendingWrites(pending map[string]any) {
	if len(pending) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Apply pending writes to committed state
	for key, value := range pending {
		s.mergeIntoCommitted(key, value)
	}

	// Update read snapshot to reflect applied writes
	newVersion := s.version.Add(1)
	s.readSnapshot = make(map[string]any, len(s.committed))
	maps.Copy(s.readSnapshot, s.committed)
	s.snapshotVersion.Store(newVersion)
	// Update cached view atomically for lock-free reads
	s.cachedView.Store(&stateView{data: s.readSnapshot, managed: s.managedValues})
}
