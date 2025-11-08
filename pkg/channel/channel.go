package channel

import (
	"context"
	"sync"
	"sync/atomic"
)

// Channel is the core abstraction for data flow between nodes.
// Each channel has specific update semantics (append, replace, merge, etc.)
//
// All channels must be cloneable to support graph state snapshots and time travel.
type Channel interface {
	// Name returns the unique identifier for this channel
	Name() string

	// Read returns the current value of the channel
	Read(ctx context.Context) (any, error)

	// Write adds value(s) to the channel using channel-specific semantics
	Write(ctx context.Context, value any) error

	// Snapshot returns a read-only snapshot of the current channel state
	// for consistent reads during a superstep
	Snapshot(ctx context.Context) (any, error)

	// Version returns the current version number (incremented on each write)
	Version() int64

	// Reset clears the channel to its initial state
	Reset(ctx context.Context) error

	// Clone creates a deep copy of this channel with independent state
	Clone() Channel
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
		tc.values = append(tc.values, v...)
	default:
		tc.values = append(tc.values, v)
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

func (tc *TopicChannel) Snapshot(ctx context.Context) (any, error) {
	return tc.Read(ctx)
}

func (tc *TopicChannel) Version() int64 {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.version
}

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
func (tc *TopicChannel) Clone() Channel {
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

func (lvc *LastValueChannel) Name() string {
	return lvc.name
}

func (lvc *LastValueChannel) Read(ctx context.Context) (any, error) {
	if !lvc.hasValue.Load() {
		return nil, nil
	}
	return lvc.value.Load(), nil
}

func (lvc *LastValueChannel) Write(ctx context.Context, value any) error {
	lvc.value.Store(value)
	lvc.hasValue.Store(true)
	lvc.version.Add(1)
	return nil
}

func (lvc *LastValueChannel) Snapshot(ctx context.Context) (any, error) {
	return lvc.Read(ctx)
}

func (lvc *LastValueChannel) Version() int64 {
	return lvc.version.Load()
}

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
func (lvc *LastValueChannel) Clone() Channel {
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

func (boc *BinaryOpChannel) Snapshot(ctx context.Context) (any, error) {
	return boc.Read(ctx)
}

func (boc *BinaryOpChannel) Version() int64 {
	return boc.version.Load()
}

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
func (boc *BinaryOpChannel) Clone() Channel {
	clone := &BinaryOpChannel{
		name:     boc.name,
		operator: boc.operator,
	}
	clone.value.Store(boc.value.Load())
	clone.version.Store(boc.version.Load())
	return clone
}

// ChannelSet manages a collection of named channels.
type ChannelSet struct {
	channels map[string]Channel
	mu       sync.RWMutex
}

// NewChannelSet creates a new channel set.
func NewChannelSet() *ChannelSet {
	return &ChannelSet{
		channels: make(map[string]Channel),
	}
}

// Add registers a channel in the set.
func (cs *ChannelSet) Add(channel Channel) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.channels[channel.Name()] = channel
}

// Get retrieves a channel by name.
func (cs *ChannelSet) Get(name string) (Channel, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ch, ok := cs.channels[name]
	return ch, ok
}

// List returns all channel names.
func (cs *ChannelSet) List() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	names := make([]string, 0, len(cs.channels))
	for name := range cs.channels {
		names = append(names, name)
	}
	return names
}

// ReadAll returns a snapshot of all channel values.
func (cs *ChannelSet) ReadAll(ctx context.Context) (map[string]any, error) {
	return cs.processAll(ctx, func(ch Channel) (any, error) {
		return ch.Read(ctx)
	})
}

// SnapshotAll returns a consistent snapshot of all channel values.
func (cs *ChannelSet) SnapshotAll(ctx context.Context) (map[string]any, error) {
	return cs.processAll(ctx, func(ch Channel) (any, error) {
		return ch.Snapshot(ctx)
	})
}

// processAll is a helper that processes all channels with the given function.
func (cs *ChannelSet) processAll(ctx context.Context, fn func(Channel) (any, error)) (map[string]any, error) {
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
func (cs *ChannelSet) WriteAll(ctx context.Context, updates map[string]any) error {
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
