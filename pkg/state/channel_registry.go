package state

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/state/internal/channel"
)

// ChannelBehavior defines how a channel handles new values.
type ChannelBehavior int

const (
	// LastValueBehavior keeps only the most recent value (replaces on write).
	LastValueBehavior ChannelBehavior = iota
	// TopicBehavior appends all values (queue/log semantics).
	TopicBehavior
	// BinaryOpBehavior applies a binary operation when combining values.
	BinaryOpBehavior
	// AggregateBehavior combines values using an aggregator (sum, max, avg, etc.).
	AggregateBehavior
)

// ChannelMetadata holds configuration for a registered channel.
type ChannelMetadata struct {
	Behavior ChannelBehavior
	Channel  channel.Channel
}

// ChannelRegistry manages channels with different semantic behaviors.
// This is the core storage layer for the unified state system.
//
// Performance: sync.Map enables concurrent reads for BSP execution where
// all workers read state simultaneously at superstep boundaries.
type ChannelRegistry struct {
	channels sync.Map   // map[string]*ChannelMetadata
	mu       sync.Mutex // For operations requiring consistency across multiple channels
}

// NewChannelRegistry creates a new channel registry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{}
}

// GetOrCreateChannel retrieves an existing channel or creates a new one.
// Default behavior is LastValueBehavior.
func (r *ChannelRegistry) GetOrCreateChannel(name string) channel.Channel {
	// Fast path: lock-free read
	if val, ok := r.channels.Load(name); ok {
		if meta, ok := val.(*ChannelMetadata); ok && meta != nil {
			return meta.Channel
		}
	}

	// Slow path: create new channel with lock
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := r.channels.Load(name); ok {
		if meta, ok := val.(*ChannelMetadata); ok && meta != nil {
			return meta.Channel
		}
	}

	// Create new channel with default LastValue behavior
	ch := channel.NewLastValueChannel(name)
	meta := &ChannelMetadata{
		Behavior: LastValueBehavior,
		Channel:  ch,
	}
	r.channels.Store(name, meta)

	return ch
}

// SetChannelBehavior configures the behavior for a named channel.
// If the channel doesn't exist, it will be created with the specified behavior.
// If it exists, the behavior is updated (but the channel instance remains).
func (r *ChannelRegistry) SetChannelBehavior(name string, behavior ChannelBehavior) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	val, exists := r.channels.Load(name)
	if !exists {
		// Create new channel with specified behavior
		var ch channel.Channel
		switch behavior {
		case LastValueBehavior:
			ch = channel.NewLastValueChannel(name)
		case TopicBehavior:
			ch = channel.NewTopicChannel(name, 0) // 0 = unlimited
		case BinaryOpBehavior:
			// BinaryOp channels require a reducer function at creation time
			// For now, we'll create a LastValue channel and let the caller
			// replace it with a properly configured BinaryOp channel
			return fmt.Errorf("BinaryOpBehavior requires explicit channel creation with reducer function")
		default:
			return fmt.Errorf("unknown channel behavior: %d", behavior)
		}

		meta := &ChannelMetadata{
			Behavior: behavior,
			Channel:  ch,
		}
		r.channels.Store(name, meta)
		return nil
	}

	// Update behavior for existing channel
	meta := val.(*ChannelMetadata)
	meta.Behavior = behavior
	return nil
}

// RegisterChannel explicitly registers a channel with specific behavior.
// This allows custom channel implementations (e.g., BinaryOp with custom reducer).
func (r *ChannelRegistry) RegisterChannel(name string, ch channel.Channel, behavior ChannelBehavior) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels.Load(name); exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	meta := &ChannelMetadata{
		Behavior: behavior,
		Channel:  ch,
	}
	r.channels.Store(name, meta)
	return nil
}

// GetChannel retrieves a channel by name.
// Returns nil if the channel doesn't exist.
//
// Performance: Lock-free read using sync.Map.Load().
func (r *ChannelRegistry) GetChannel(name string) channel.Channel {
	val, ok := r.channels.Load(name)
	if !ok {
		return nil
	}
	if meta, ok := val.(*ChannelMetadata); ok && meta != nil {
		return meta.Channel
	}
	return nil
}

// GetChannelMetadata retrieves metadata for a channel.
// Returns nil if the channel doesn't exist.
//
// Performance: Lock-free read using sync.Map.Load().
func (r *ChannelRegistry) GetChannelMetadata(name string) *ChannelMetadata {
	val, ok := r.channels.Load(name)
	if !ok {
		return nil
	}
	return val.(*ChannelMetadata)
}

// GetChannelValue reads the current value from a channel.
// For LastValue/BinaryOp channels, returns the single value.
// For Topic channels, returns all values as a slice.
// Returns nil if channel doesn't exist or is empty.
//
// Performance: Lock-free channel lookup, eliminates RWMutex contention
// for concurrent reads during BSP superstep execution.
func (r *ChannelRegistry) GetChannelValue(ctx context.Context, name string) (any, error) {
	val, ok := r.channels.Load(name)
	if !ok {
		return nil, fmt.Errorf("channel %q not found", name)
	}

	meta := val.(*ChannelMetadata)
	return meta.Channel.Read(ctx)
}

// WriteValue writes a value to a channel, respecting its behavior.
// For LastValue channels, replaces the current value.
// For Topic channels, appends to the queue.
// For BinaryOp channels, applies the reducer function.
func (r *ChannelRegistry) WriteValue(ctx context.Context, name string, value any) error {
	ch := r.GetChannel(name)
	if ch == nil {
		return fmt.Errorf("channel %q not found", name)
	}

	return ch.Write(ctx, value)
}

// DeleteChannel removes a channel from the registry.
// Returns an error if the channel doesn't exist.
func (r *ChannelRegistry) DeleteChannel(name string) error {
	_, loaded := r.channels.LoadAndDelete(name)
	if !loaded {
		return fmt.Errorf("channel %q not found", name)
	}
	return nil
}

// Channels returns a list of all registered channel names.
func (r *ChannelRegistry) Channels() []string {
	var names []string
	r.channels.Range(func(key, value any) bool {
		names = append(names, key.(string))
		return true
	})
	return names
}

// Snapshot creates a snapshot of all channel values.
// For LastValue/BinaryOp channels, captures the single value.
// For Topic channels, captures all queued values as a slice.
func (r *ChannelRegistry) Snapshot(ctx context.Context) (map[string]any, error) {
	snapshot := make(map[string]any)

	r.channels.Range(func(key, value any) bool {
		name := key.(string)
		meta := value.(*ChannelMetadata)

		val, err := meta.Channel.Read(ctx)
		if err != nil {
			// Skip channels that can't be read
			return true
		}
		snapshot[name] = val
		return true
	})

	return snapshot, nil
}

// Restore loads values from a snapshot into channels.
// Creates channels if they don't exist (using LastValue behavior by default).
func (r *ChannelRegistry) Restore(ctx context.Context, snapshot map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, value := range snapshot {
		val, exists := r.channels.Load(name)

		var meta *ChannelMetadata
		if !exists {
			// Create new channel with LastValue behavior
			ch := channel.NewLastValueChannel(name)
			meta = &ChannelMetadata{
				Behavior: LastValueBehavior,
				Channel:  ch,
			}
			r.channels.Store(name, meta)
		} else {
			meta = val.(*ChannelMetadata)
		}

		// Write value to channel
		if value != nil {
			if err := meta.Channel.Write(ctx, value); err != nil {
				return fmt.Errorf("failed to restore channel %q: %w", name, err)
			}
		}
	}

	return nil
}

// Clear removes all channels from the registry.
func (r *ChannelRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear all entries from sync.Map
	r.channels.Range(func(key, value any) bool {
		r.channels.Delete(key)
		return true
	})
}

// Len returns the number of registered channels.
func (r *ChannelRegistry) Len() int {
	count := 0
	r.channels.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}
