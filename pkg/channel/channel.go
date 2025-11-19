package channel

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
)

var (
	// ErrNilValue is returned when attempting to write a nil value to a LastValueChannel.
	// LastValueChannel uses atomic.Value which cannot store nil values.
	ErrNilValue = errors.New("channel: cannot write nil value to LastValueChannel")
)

// SliceValue is an interface for values that can be treated as slices.
// Implement this interface to provide slice semantics without reflection.
//
// This eliminates the need for reflection when writing slices to TopicChannel,
// improving performance and type safety.
//
// Example:
//
//	type Messages []message.Message
//
//	func (m Messages) ToSlice() []any {
//	    result := make([]any, len(m))
//	    for i, msg := range m {
//	        result[i] = msg
//	    }
//	    return result
//	}
//
//	// Usage with TopicChannel
//	ch.Write(ctx, Messages{msg1, msg2, msg3})
type SliceValue interface {
	ToSlice() []any
}

// SliceOf is a generic helper that wraps any slice type to implement SliceValue.
// This provides a convenient way to write strongly-typed slices to channels
// without implementing SliceValue for every type.
//
// Example:
//
//	messages := []message.Message{msg1, msg2}
//	ch.Write(ctx, SliceOf(messages))
type SliceOf[T any] []T

// ToSlice converts the strongly-typed slice to []any for channel storage.
func (s SliceOf[T]) ToSlice() []any {
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}

// Channel is the user-facing abstraction for data flow between nodes.
// Provides simple read/write operations with channel-specific update semantics.
//
// Design: This interface exposes only essential operations needed by graph nodes.
// Internal runtime operations (versioning, snapshots, cloning) are handled by
// VersionedChannel to prevent misuse and clarify API boundaries.
type Channel interface {
	// Name returns the unique identifier for this channel
	Name() string

	// Read returns the current value of the channel
	Read(ctx context.Context) (any, error)

	// Write adds value(s) to the channel using channel-specific semantics
	// (append for TopicChannel, replace for LastValueChannel, merge for BinaryOpChannel)
	Write(ctx context.Context, value any) error
}

// VersionedChannel extends Channel with internal runtime operations.
// Used by the graph execution engine for cache invalidation, consistent snapshots,
// and state cloning during checkpointing.
//
// Design: These methods are implementation details that should not be exposed
// to user code. Separating them prevents accidental misuse and makes the API
// clearer about what operations are safe for general use vs internal use.
type VersionedChannel interface {
	Channel

	// Version returns the current version number (incremented on each write).
	// Used for cache invalidation in copy-on-write optimizations.
	// Internal use only - not part of public Channel API.
	Version() int64

	// Snapshot returns a read-only snapshot of the current channel state
	// for consistent reads during a superstep. Typically equivalent to Read()
	// but semantically indicates a point-in-time capture.
	// Internal use only - used by graph runtime for superstep isolation.
	Snapshot(ctx context.Context) (any, error)

	// Clone creates a deep copy of this channel with independent state.
	// Used during state checkpointing and time travel to create isolated copies.
	// Returns VersionedChannel to preserve internal operations on cloned instances.
	// Internal use only - deep copy semantics require careful memory management.
	Clone() VersionedChannel
}

// ResettableChannel extends Channel with administrative operations.
// Provides dangerous state-clearing operations that should only be used
// with explicit understanding of the consequences.
//
// Design: Reset() can corrupt state if called incorrectly during graph execution.
// By segregating this operation into a separate interface, we make it explicit
// that this is an administrative operation requiring careful consideration.
//
// Warning: Reset() during active graph execution may cause data loss, race
// conditions, or inconsistent state. Only use in controlled scenarios:
//   - Between graph runs (not during execution)
//   - In test cleanup code
//   - During explicit state reinitialization
type ResettableChannel interface {
	Channel

	// Reset clears the channel to its initial state.
	// WARNING: This is a destructive operation. Calling Reset() during graph
	// execution can cause data loss and state corruption. Only use when you
	// have explicit control over the execution lifecycle.
	Reset(ctx context.Context) error
}

// TopicChannel accumulates values in append-only fashion.
// Values are never removed, only appended. Optional maxValues limit enforces retention policy.
//
// Optimization: Uses copy-on-write caching to avoid redundant slice copies when reading
// the same data multiple times. The cached snapshot is invalidated on writes via version tracking.
type TopicChannel struct {
	name      string
	values    []any
	version   int64
	mu        sync.RWMutex
	maxValues int // 0 means unlimited

	// Copy-on-write cache for Read() operations
	cachedSnapshot []any
	cachedVersion  int64
}

// NewTopicChannel creates a new topic channel that accumulates values.
func NewTopicChannel(name string, maxValues int) *TopicChannel {
	return &TopicChannel{
		name:      name,
		values:    make([]any, 0),
		maxValues: maxValues,
	}
}

// Name returns the channel's identifier.
func (tc *TopicChannel) Name() string {
	return tc.name
}

func (tc *TopicChannel) Read(ctx context.Context) (any, error) {
	// Fast path: check if cached snapshot is valid (read lock only)
	tc.mu.RLock()
	if tc.cachedVersion == tc.version && tc.cachedSnapshot != nil {
		snapshot := tc.cachedSnapshot
		tc.mu.RUnlock()
		return snapshot, nil
	}
	tc.mu.RUnlock()

	// Slow path: create new snapshot (write lock to update cache)
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have updated)
	if tc.cachedVersion == tc.version && tc.cachedSnapshot != nil {
		return tc.cachedSnapshot, nil
	}

	// Create snapshot and cache it
	snapshot := make([]any, len(tc.values))
	copy(snapshot, tc.values)
	tc.cachedSnapshot = snapshot
	tc.cachedVersion = tc.version

	return snapshot, nil
}

func (tc *TopicChannel) Write(ctx context.Context, value any) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Support both single values and slices
	switch v := value.(type) {
	case []any:
		// Fast path for []any
		tc.values = append(tc.values, v...)
	case SliceValue:
		// Interface-based slice handling (no reflection needed)
		tc.values = append(tc.values, v.ToSlice()...)
	default:
		// Fallback: check if it's a typed slice using reflection
		// This handles cases like []message.Message, []string, etc.
		if value != nil {
			rv := reflect.ValueOf(value)
			if rv.Kind() == reflect.Slice {
				// Convert typed slice to []any
				for i := 0; i < rv.Len(); i++ {
					tc.values = append(tc.values, rv.Index(i).Interface())
				}
			} else {
				// Single non-slice value (string, int, struct, etc.)
				tc.values = append(tc.values, value)
			}
		} else {
			// Single nil value
			tc.values = append(tc.values, value)
		}
	}

	// Enforce max values limit if set
	if tc.maxValues > 0 && len(tc.values) > tc.maxValues {
		// Keep most recent values
		tc.values = tc.values[len(tc.values)-tc.maxValues:]
	}

	tc.version++
	// Invalidate cache on write (version mismatch will trigger new snapshot)
	tc.cachedSnapshot = nil
	return nil
}

// Snapshot returns a copy of all accumulated values.
func (tc *TopicChannel) Snapshot(ctx context.Context) (any, error) {
	return tc.Read(ctx)
}

// Version returns the current version number for cache invalidation.
func (tc *TopicChannel) Version() int64 {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.version
}

// Reset clears all accumulated values and resets version.
func (tc *TopicChannel) Reset(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.values = make([]any, 0)
	tc.version = 0
	tc.cachedSnapshot = nil
	tc.cachedVersion = 0
	return nil
}

// MaxValues returns the current retention limit configured on the topic.
func (tc *TopicChannel) MaxValues() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.maxValues
}

// SetMaxValues updates the retention limit and truncates values if necessary.
func (tc *TopicChannel) SetMaxValues(limit int) {
	if limit < 0 {
		limit = 0
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.maxValues = limit
	if limit > 0 && len(tc.values) > limit {
		tc.values = append([]any(nil), tc.values[len(tc.values)-limit:]...)
		tc.version++
		tc.cachedSnapshot = nil // Invalidate cache
	}
}

// Clone returns a deep copy of the topic channel.
func (tc *TopicChannel) Clone() VersionedChannel {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	cloneValues := make([]any, len(tc.values))
	copy(cloneValues, tc.values)

	return &TopicChannel{
		name:      tc.name,
		values:    cloneValues,
		version:   tc.version,
		maxValues: tc.maxValues,
		// Don't copy cache - let clone build its own
		cachedSnapshot: nil,
		cachedVersion:  0,
	}
}

// LastValueChannel stores only the most recent value (overwrite semantics).
// Each update replaces the previous value completely.
//
// Thread-safety: Uses atomic.Value for lock-free reads and atomic operations
// for version tracking. This eliminates the data race that occurred with the
// previous readCached implementation.
//
// Nil Value Handling: Write() returns ErrNilValue if attempting to store nil.
// This is a limitation of atomic.Value which cannot store nil values in Go.
// To clear a value, delete the channel instead or use a sentinel value pattern.
type LastValueChannel struct {
	name     string
	value    atomic.Value // Stores the actual value
	version  atomic.Int64 // Version counter
	hasValue atomic.Bool  // Tracks if value has been set
}

// NewLastValueChannel creates a new last-value channel with overwrite semantics.
func NewLastValueChannel(name string) *LastValueChannel {
	return &LastValueChannel{
		name: name,
	}
}

// Name returns the channel's identifier.
func (lvc *LastValueChannel) Name() string {
	return lvc.name
}

func (lvc *LastValueChannel) Read(ctx context.Context) (any, error) {
	if !lvc.hasValue.Load() {
		return nil, nil
	}
	return lvc.value.Load(), nil
}

// Write stores a new value, replacing any previous value.
// Returns ErrNilValue if value is nil, as atomic.Value cannot store nil.
func (lvc *LastValueChannel) Write(ctx context.Context, value any) error {
	if value == nil {
		return ErrNilValue
	}
	lvc.value.Store(value)
	lvc.hasValue.Store(true)
	lvc.version.Add(1)
	return nil
}

// Snapshot returns the current value.
func (lvc *LastValueChannel) Snapshot(ctx context.Context) (any, error) {
	return lvc.Read(ctx)
}

// Version returns the current version number.
func (lvc *LastValueChannel) Version() int64 {
	return lvc.version.Load()
}

// Reset marks the channel as having no value.
func (lvc *LastValueChannel) Reset(ctx context.Context) error {
	// atomic.Value doesn't allow storing nil, so we just mark as not having a value
	// The old value remains in memory but is inaccessible via Read()
	lvc.hasValue.Store(false)
	lvc.version.Store(0)
	return nil
}

// HasValue returns true if the channel has been written to at least once.
func (lvc *LastValueChannel) HasValue() bool {
	return lvc.hasValue.Load()
}

// Clone returns a deep copy of the last value channel.
func (lvc *LastValueChannel) Clone() VersionedChannel {
	clone := &LastValueChannel{
		name: lvc.name,
	}
	if lvc.hasValue.Load() {
		clone.value.Store(lvc.value.Load())
		clone.hasValue.Store(true)
		clone.version.Store(lvc.version.Load())
	}
	return clone
}

// BinaryOpChannel applies a binary operator to combine values.
// Updates are merged with the current value using a custom operator function.
//
// Thread-safety: Uses atomic.Value for the current value and a mutex only
// for the write operation (since we need to read-modify-write atomically).
// Reads are lock-free.
type BinaryOpChannel struct {
	name     string
	value    atomic.Value // Stores the current combined value
	operator func(current, incoming any) any
	version  atomic.Int64
	mu       sync.Mutex // Only for write operations (read-modify-write)
}

// NewBinaryOpChannel creates a channel that combines values using the given operator.
func NewBinaryOpChannel(name string, initialValue any, op func(current, incoming any) any) *BinaryOpChannel {
	boc := &BinaryOpChannel{
		name:     name,
		operator: op,
	}
	boc.value.Store(initialValue)
	return boc
}

// Name returns the channel's identifier.
func (boc *BinaryOpChannel) Name() string {
	return boc.name
}

func (boc *BinaryOpChannel) Read(ctx context.Context) (any, error) {
	return boc.value.Load(), nil
}

func (boc *BinaryOpChannel) Write(ctx context.Context, value any) error {
	// Need mutex for read-modify-write atomicity
	boc.mu.Lock()
	defer boc.mu.Unlock()

	current := boc.value.Load()
	newValue := boc.operator(current, value)
	boc.value.Store(newValue)
	boc.version.Add(1)
	return nil
}

// Snapshot returns the current combined value.
func (boc *BinaryOpChannel) Snapshot(ctx context.Context) (any, error) {
	return boc.Read(ctx)
}

// Version returns the current version number.
func (boc *BinaryOpChannel) Version() int64 {
	return boc.version.Load()
}

// Reset clears the channel's state (implementation-specific behavior).
func (boc *BinaryOpChannel) Reset(ctx context.Context) error {
	boc.mu.Lock()
	defer boc.mu.Unlock()

	// Reset to operator's zero value by applying to nil
	resetValue := boc.operator(nil, nil)
	boc.value.Store(resetValue)
	boc.version.Store(0)
	return nil
}

// Clone returns a deep copy of the binary op channel.
func (boc *BinaryOpChannel) Clone() VersionedChannel {
	clone := &BinaryOpChannel{
		name:     boc.name,
		operator: boc.operator,
	}
	clone.value.Store(boc.value.Load())
	clone.version.Store(boc.version.Load())
	return clone
}

// Set manages a collection of named channels for coordinated state management.
type Set struct {
	channels map[string]Channel
	mu       sync.RWMutex
}

// NewSet creates a new channel set.
func NewSet() *Set {
	return &Set{
		channels: make(map[string]Channel),
	}
}

// Add registers a channel in the set.
func (cs *Set) Add(channel Channel) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.channels[channel.Name()] = channel
}

// Get retrieves a channel by name.
func (cs *Set) Get(name string) (Channel, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ch, ok := cs.channels[name]
	return ch, ok
}

// List returns all channel names.
func (cs *Set) List() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	names := make([]string, 0, len(cs.channels))
	for name := range cs.channels {
		names = append(names, name)
	}
	return names
}

// ReadAll returns a snapshot of all channel values.
func (cs *Set) ReadAll(ctx context.Context) (map[string]any, error) {
	return cs.processAll(func(ch Channel) (any, error) {
		return ch.Read(ctx)
	})
}

// SnapshotAll returns a consistent snapshot of all channel values.
// Uses VersionedChannel.Snapshot() for internal runtime operations.
func (cs *Set) SnapshotAll(ctx context.Context) (map[string]any, error) {
	return cs.processAll(func(ch Channel) (any, error) {
		// Type assert to VersionedChannel for snapshot access
		if vch, ok := ch.(VersionedChannel); ok {
			return vch.Snapshot(ctx)
		}
		// Fallback to Read() if channel doesn't support Snapshot()
		return ch.Read(ctx)
	})
}

// processAll is a helper that processes all channels with the given function.
func (cs *Set) processAll(fn func(Channel) (any, error)) (map[string]any, error) {
	cs.mu.RLock()
	channels := make([]Channel, 0, len(cs.channels))
	for _, ch := range cs.channels {
		channels = append(channels, ch)
	}
	cs.mu.RUnlock()

	result := make(map[string]any, len(channels))
	for _, ch := range channels {
		value, err := fn(ch)
		if err != nil {
			return nil, err
		}
		result[ch.Name()] = value
	}
	return result, nil
}

// WriteAll writes values to multiple channels.
func (cs *Set) WriteAll(ctx context.Context, updates map[string]any) error {
	for name, value := range updates {
		ch, ok := cs.Get(name)
		if !ok {
			// Skip unknown channels (allows nodes to write to optional channels)
			continue
		}
		if err := ch.Write(ctx, value); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Interface Assertions - Compile-time verification
// =============================================================================

// Verify all channel implementations satisfy the interface hierarchy
var (
	_ Channel           = (*TopicChannel)(nil)
	_ VersionedChannel  = (*TopicChannel)(nil)
	_ ResettableChannel = (*TopicChannel)(nil)
	_ Channel           = (*LastValueChannel)(nil)
	_ VersionedChannel  = (*LastValueChannel)(nil)
	_ ResettableChannel = (*LastValueChannel)(nil)
	_ Channel           = (*BinaryOpChannel)(nil)
	_ VersionedChannel  = (*BinaryOpChannel)(nil)
	_ ResettableChannel = (*BinaryOpChannel)(nil)
)
