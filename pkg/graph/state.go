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
type StateKey interface {
	// Name returns the unique name of this key.
	Name() string

	// ReducerFunc returns the type-erased reducer for this key.
	ReducerFunc() ReducerFunc

	// stateKey is a marker method to prevent external implementations.
	stateKey()
}

// Key defines a typed state channel with an associated reducer.
// The reducer determines how values are merged during state updates.
type Key[T any] struct {
	name      string
	reducerFn ReducerFunc // type-erased reducer (set at construction)
}

// stateKey implements StateKey.
func (Key[T]) stateKey() {}

// KeyOption configures a Key.
type KeyOption[T any] func(*Key[T])

// WithReducer sets a custom reducer for the key.
func WithReducer[T any](r Reducer[T]) KeyOption[T] {
	return func(k *Key[T]) {
		k.reducerFn = WrapReducer(r)
	}
}

// NewKey creates a state key with Replace (overwrite) semantics by default.
func NewKey[T any](name string, opts ...KeyOption[T]) Key[T] {
	k := Key[T]{
		name:      name,
		reducerFn: WrapReducer(ReplaceReducer[T]{}),
	}

	for _, opt := range opts {
		opt(&k)
	}

	return k
}

// NewListKey creates a list state key with Append semantics by default.
// The key stores []T and appends incoming slices.
func NewListKey[T any](name string, opts ...KeyOption[[]T]) Key[[]T] {
	k := Key[[]T]{
		name:      name,
		reducerFn: WrapReducer(AppendReducer[T]{}),
	}

	for _, opt := range opts {
		opt(&k)
	}

	return k
}

// NewCounterKey creates a counter key with Sum semantics.
func NewCounterKey(name string, opts ...KeyOption[int]) Key[int] {
	k := Key[int]{
		name:      name,
		reducerFn: WrapReducer(SumReducer[int]{}),
	}

	for _, opt := range opts {
		opt(&k)
	}

	return k
}

// NewMapKey creates a map key with MergeMap semantics.
func NewMapKey[K comparable, V any](name string, opts ...KeyOption[map[K]V]) Key[map[K]V] {
	k := Key[map[K]V]{
		name:      name,
		reducerFn: WrapReducer(MergeMapReducer[K, V]{}),
	}

	for _, opt := range opts {
		opt(&k)
	}

	return k
}

// Name returns the key name.
func (k Key[T]) Name() string {
	return k.name
}

// ReducerFunc returns the type-erased reducer for runtime use.
func (k Key[T]) ReducerFunc() ReducerFunc {
	return k.reducerFn
}

// Zero returns the zero value for this key's type.
func (k Key[T]) Zero() T {
	return k.reducerFn.ZeroFn().(T)
}

// Get returns the typed value for a key from the scope.
// If no value exists, returns the reducer's zero value.
func Get[T any](scope ReadOnlyScope, key Key[T]) T {
	if v, ok := scope.GetValue(key.name); ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}

	return key.reducerFn.ZeroFn().(T)
}

// GetList returns the typed slice for a list key from the scope.
// If no value exists, returns nil.
func GetList[T any](scope ReadOnlyScope, key Key[[]T]) []T {
	if v, ok := scope.GetValue(key.name); ok {
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

// -----------------------------------------------------------------------------
// BSP State Manager - Optimized Bulk-Synchronous Parallel semantics
// -----------------------------------------------------------------------------

// KeyRegistry holds type-erased reducers for all registered keys.
type KeyRegistry map[string]ReducerFunc

// NewKeyRegistry creates a new empty KeyRegistry.
func NewKeyRegistry() KeyRegistry {
	return make(KeyRegistry)
}

// Register adds a reducer for a key name.
func (r KeyRegistry) Register(name string, reducer ReducerFunc) {
	r[name] = reducer
}

// BSPState manages state with proper BSP semantics:
// - All reads within a superstep see the same snapshot (from previous superstep)
// - All writes are buffered and only become visible after barrier commit
// - This ensures deterministic parallel execution regardless of scheduling order
//
// Optimizations:
// - Copy-on-write: readSnapshot only recreated when writes occur
// - Version tracking: avoid unnecessary snapshot copies
// - Reducer-based merging: uses registered reducers for state updates
// - Atomic version checking: skip locking in ReadView when possible
type BSPState struct {
	mu sync.RWMutex

	// keyRegistry holds type-erased reducers for all registered keys.
	keyRegistry KeyRegistry

	// readSnapshot is the immutable state visible to all nodes in current superstep.
	// Created at superstep start from committed state.
	// Protected by mu for writes, but can be read atomically via cachedView.
	readSnapshot map[string]any

	// writeBuffer accumulates all writes during current superstep.
	// Writes are merged using reducers.
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
	cachedView atomic.Pointer[readOnlyScope]

	// managedValues holds ephemeral runtime state (not checkpointed).
	managedValues *ManagedValueRegistry

	// pendingWrites captures provenance information for two-phase commit checkpoints.
	// Each entry records which node wrote to which channel along with the raw value.
	pendingWrites []checkpoint.PendingWrite
}

// NewBSPState creates a new BSP-compliant state manager.
func NewBSPState(initial map[string]any, keyRegistry KeyRegistry) *BSPState {
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
		keyRegistry:   keyRegistry,
		readSnapshot:  committed, // Share initially (CoW)
		writeBuffer:   make(map[string]any),
		committed:     committed,
		ownsCommitted: ownedCommitted,
	}
	// version and snapshotVersion start at 0 (zero value)
	// Set initial cached view
	state.cachedView.Store(&readOnlyScope{data: committed})

	return state
}

// setManagedValues attaches a managed values registry to the state (internal).
func (s *BSPState) setManagedValues(registry *ManagedValueRegistry) {
	s.managedValues = registry
	// Update cached view to include managed values
	s.mu.RLock()
	s.cachedView.Store(&readOnlyScope{data: s.readSnapshot, managed: registry})
	s.mu.RUnlock()
}

// ReadView returns a ReadOnlyScope that reads from the current superstep's snapshot.
// This scope is safe for concurrent reads - it reads from immutable snapshot.
// Optimized: uses atomic pointer to cached view, avoiding lock acquisition
// when snapshot hasn't changed.
func (s *BSPState) ReadView() ReadOnlyScope {
	// Fast path: return cached view without locking
	// The cached view is atomically updated whenever readSnapshot changes
	if cached := s.cachedView.Load(); cached != nil {
		return cached
	}

	// Fallback: create view under lock (should rarely happen)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &readOnlyScope{data: s.readSnapshot, managed: s.managedValues}
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

// mergeWrite merges a value into the write buffer using the key's reducer.
// If a reducer is registered for the key, it is used to merge values.
// Otherwise, falls back to legacy mergeSlices behavior.
// Must be called while holding s.mu lock.
func (s *BSPState) mergeWrite(key string, value any) {
	existing, exists := s.writeBuffer[key]

	// If reducer is registered, use it
	if reducer, ok := s.keyRegistry[key]; ok {
		if !exists {
			existing = reducer.ZeroFn()
		}

		s.writeBuffer[key] = reducer.ReduceFn(existing, value)

		return
	}

	// Legacy fallback: no reducer registered
	// Fast path: check common non-slice types without reflection
	switch value.(type) {
	case string, int, int64, float64, bool, nil:
		s.writeBuffer[key] = value
		return
	}

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
	s.cachedView.Store(&readOnlyScope{data: s.readSnapshot, managed: s.managedValues})
}

// mergeIntoCommitted merges a value from write buffer into committed state.
// Uses the key's reducer if registered, otherwise falls back to legacy behavior.
// Must be called while holding s.mu lock.
func (s *BSPState) mergeIntoCommitted(key string, value any) {
	s.ensureCommittedOwnership()

	existing, exists := s.committed[key]

	// If reducer is registered, use it
	if reducer, ok := s.keyRegistry[key]; ok {
		if !exists {
			existing = reducer.ZeroFn()
		}

		s.committed[key] = reducer.ReduceFn(existing, value)

		return
	}

	// Legacy fallback: no reducer registered
	// Fast path for common scalar types
	switch value.(type) {
	case string, int, int64, float64, bool, nil:
		s.committed[key] = value
		return
	}

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
	s.cachedView.Store(&readOnlyScope{data: s.readSnapshot, managed: s.managedValues})
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
