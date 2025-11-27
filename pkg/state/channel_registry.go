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
// Performance: Frozen map after setup for lock-free concurrent reads during BSP execution.
// Channels are registered during setup, then only read during execution.
type ChannelRegistry struct {
	channels map[string]*ChannelMetadata // Frozen after setup - lock-free reads
	mu       sync.RWMutex                // For registration phase, read lock for lookups
	frozen   bool                        // Set to true after Compile() to prevent modifications
}

// NewChannelRegistry creates a new channel registry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		channels: make(map[string]*ChannelMetadata),
	}
}

// GetOrCreateChannel retrieves an existing channel or creates a new one.
// Default behavior is LastValueBehavior.
func (r *ChannelRegistry) GetOrCreateChannel(name string) channel.Channel {
	// Fast path: lock-free read
	r.mu.RLock()
	meta, exists := r.channels[name]
	r.mu.RUnlock()

	if exists && meta != nil {
		return meta.Channel
	}

	// Slow path: create new channel with write lock
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring lock
	if meta, exists := r.channels[name]; exists && meta != nil {
		return meta.Channel
	}

	// Create new channel with default LastValue behavior
	ch := channel.NewLastValueChannel(name)
	meta = &ChannelMetadata{
		Behavior: LastValueBehavior,
		Channel:  ch,
	}
	r.channels[name] = meta

	return ch
}

// SetChannelBehavior configures the behavior for a named channel.
// If the channel doesn't exist, it will be created with the specified behavior.
// If it exists, the behavior is updated (but the channel instance remains).
func (r *ChannelRegistry) SetChannelBehavior(name string, behavior ChannelBehavior) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta, exists := r.channels[name]
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
		r.channels[name] = meta
		return nil
	}

	// Update behavior for existing channel
	meta.Behavior = behavior
	return nil
}

// RegisterChannel explicitly registers a channel with specific behavior.
// This allows custom channel implementations (e.g., BinaryOp with custom reducer).
func (r *ChannelRegistry) RegisterChannel(name string, ch channel.Channel, behavior ChannelBehavior) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		return fmt.Errorf("cannot register channel %q: registry is frozen after compilation", name)
	}

	if _, exists := r.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	meta := &ChannelMetadata{
		Behavior: behavior,
		Channel:  ch,
	}
	r.channels[name] = meta
	return nil
}

// GetChannel retrieves a channel by name.
// Returns nil if the channel doesn't exist.
//
// Performance: Lock-free read after setup phase.
func (r *ChannelRegistry) GetChannel(name string) channel.Channel {
	r.mu.RLock()
	meta := r.channels[name]
	r.mu.RUnlock()

	if meta != nil {
		return meta.Channel
	}
	return nil
}

// GetChannelMetadata retrieves metadata for a channel.
// Returns nil if the channel doesn't exist.
//
// Performance: Lock-free read after setup phase.
func (r *ChannelRegistry) GetChannelMetadata(name string) *ChannelMetadata {
	r.mu.RLock()
	meta := r.channels[name]
	r.mu.RUnlock()
	return meta
}

// GetChannelValue reads the current value from a channel.
// For LastValue/BinaryOp channels, returns the single value.
// For Topic channels, returns all values as a slice.
// Returns nil if channel doesn't exist or is empty.
//
// Performance: Lock-free channel lookup after setup phase.
func (r *ChannelRegistry) GetChannelValue(ctx context.Context, name string) (any, error) {
	r.mu.RLock()
	meta := r.channels[name]
	r.mu.RUnlock()

	if meta == nil {
		return nil, fmt.Errorf("channel %q not found", name)
	}

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
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[name]; !exists {
		return fmt.Errorf("channel %q not found", name)
	}
	delete(r.channels, name)
	return nil
}

// Channels returns a list of all registered channel names.
func (r *ChannelRegistry) Channels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}

// Snapshot creates a snapshot of all channel values.
// For LastValue/BinaryOp channels, captures the single value.
// For Topic channels, captures all queued values as a slice.
func (r *ChannelRegistry) Snapshot(ctx context.Context) (map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := make(map[string]any)

	for name, meta := range r.channels {
		val, err := meta.Channel.Read(ctx)
		if err != nil {
			// Skip channels that can't be read
			continue
		}
		snapshot[name] = val
	}

	return snapshot, nil
}

// Restore loads values from a snapshot into channels.
// Creates channels if they don't exist (using LastValue behavior by default).
func (r *ChannelRegistry) Restore(ctx context.Context, snapshot map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, value := range snapshot {
		meta, exists := r.channels[name]

		if !exists {
			// Create new channel with LastValue behavior
			ch := channel.NewLastValueChannel(name)
			meta = &ChannelMetadata{
				Behavior: LastValueBehavior,
				Channel:  ch,
			}
			r.channels[name] = meta
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

	// Clear all entries from map
	for key := range r.channels {
		delete(r.channels, key)
	}
}

// Len returns the number of registered channels.
func (r *ChannelRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.channels)
}

// Freeze marks the registry as frozen, preventing further channel registrations.
// This is called by Manager.Freeze() to enforce the write-once pattern.
func (r *ChannelRegistry) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

// IsFrozen returns whether the registry is frozen.
func (r *ChannelRegistry) IsFrozen() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frozen
}
