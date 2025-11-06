// Package channel provides abstractions for typed data flow between nodes in a graph.
// Channels replace direct shared-state access with structured communication patterns.
package channel

import (
	"context"
	"sync"
)

// Channel is the core abstraction for data flow between nodes.
// Each channel has specific update semantics (append, replace, merge, etc.)
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
}

// TopicChannel accumulates values in append-only fashion.
// Values are never removed, only appended. Optional maxValues limit enforces retention policy.
type TopicChannel struct {
	name      string
	values    []any
	version   int64
	mu        sync.RWMutex
	maxValues int // 0 means unlimited
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
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	// Return a copy to prevent external mutation
	result := make([]any, len(tc.values))
	copy(result, tc.values)
	return result, nil
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
	return nil
}

// LastValueChannel stores only the most recent value (overwrite semantics).
// Each update replaces the previous value completely.
type LastValueChannel struct {
	name     string
	value    any
	version  int64
	mu       sync.RWMutex
	hasValue bool
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
	lvc.mu.RLock()
	defer lvc.mu.RUnlock()
	return lvc.value, nil
}

func (lvc *LastValueChannel) Write(ctx context.Context, value any) error {
	lvc.mu.Lock()
	defer lvc.mu.Unlock()

	lvc.value = value
	lvc.hasValue = true
	lvc.version++
	return nil
}

func (lvc *LastValueChannel) Snapshot(ctx context.Context) (any, error) {
	return lvc.Read(ctx)
}

func (lvc *LastValueChannel) Version() int64 {
	lvc.mu.RLock()
	defer lvc.mu.RUnlock()
	return lvc.version
}

func (lvc *LastValueChannel) Reset(ctx context.Context) error {
	lvc.mu.Lock()
	defer lvc.mu.Unlock()

	lvc.value = nil
	lvc.hasValue = false
	lvc.version = 0
	return nil
}

// HasValue returns true if the channel has been written to at least once.
func (lvc *LastValueChannel) HasValue() bool {
	lvc.mu.RLock()
	defer lvc.mu.RUnlock()
	return lvc.hasValue
}

// BinaryOpChannel applies a binary operator to combine values.
// Updates are merged with the current value using a custom operator function.
type BinaryOpChannel struct {
	name     string
	value    any
	operator func(current, incoming any) any
	version  int64
	mu       sync.RWMutex
}

// NewBinaryOpChannel creates a channel that combines values using the given operator.
func NewBinaryOpChannel(name string, initialValue any, op func(current, incoming any) any) *BinaryOpChannel {
	return &BinaryOpChannel{
		name:     name,
		value:    initialValue,
		operator: op,
	}
}

func (boc *BinaryOpChannel) Name() string {
	return boc.name
}

func (boc *BinaryOpChannel) Read(ctx context.Context) (any, error) {
	boc.mu.RLock()
	defer boc.mu.RUnlock()
	return boc.value, nil
}

func (boc *BinaryOpChannel) Write(ctx context.Context, value any) error {
	boc.mu.Lock()
	defer boc.mu.Unlock()

	boc.value = boc.operator(boc.value, value)
	boc.version++
	return nil
}

func (boc *BinaryOpChannel) Snapshot(ctx context.Context) (any, error) {
	return boc.Read(ctx)
}

func (boc *BinaryOpChannel) Version() int64 {
	boc.mu.RLock()
	defer boc.mu.RUnlock()
	return boc.version
}

func (boc *BinaryOpChannel) Reset(ctx context.Context) error {
	boc.mu.Lock()
	defer boc.mu.Unlock()

	// Reset to operator's zero value by applying to nil
	boc.value = boc.operator(nil, nil)
	boc.version = 0
	return nil
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
