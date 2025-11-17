package state

import (
	"context"
	"fmt"
)

// ApplyUpdates applies a map of updates to the manager.
// For registered list keys, values are appended. For regular keys, values are set/replaced.
// This is a convenience method for batch updates during graph execution.
func ApplyUpdates(ctx context.Context, m *Manager, updates map[string]any) error {
	if updates == nil {
		return nil
	}

	for key, value := range updates {
		// Check if this is a registered list key
		isListKey := m.types.IsListKey(key)

		if isListKey {
			// For list keys, append the value
			// Note: value might be a single item or a slice
			if err := appendValue(ctx, m, key, value); err != nil {
				return fmt.Errorf("failed to append to key %q: %w", key, err)
			}
		} else {
			// For regular keys, set/replace the value
			if err := setValue(ctx, m, key, value); err != nil {
				return fmt.Errorf("failed to set key %q: %w", key, err)
			}
		}
	}

	return nil
}

// setValue sets a value in the manager without type checking (internal use).
func setValue(ctx context.Context, m *Manager, key string, value any) error {
	// Write to channel registry (which writes to store)
	return m.channels.WriteValue(ctx, key, value)
}

// appendValue appends a value to a list key (internal use).
func appendValue(ctx context.Context, m *Manager, key string, value any) error {
	// Write to channel registry (which handles append semantics via TopicChannel)
	return m.channels.WriteValue(ctx, key, value)
}

// ResetInManager resets a channel to its initial empty state.
// This is used to clear list keys (TopicChannels) or reset LastValue channels.
// WARNING: This is a destructive operation. Use with caution.
func ResetInManager(ctx context.Context, m *Manager, key string) error {
	ch := m.channels.GetChannel(key)
	if ch == nil {
		return fmt.Errorf("channel %q not found", key)
	}

	// Check if channel supports Reset
	resettable, ok := ch.(interface {
		Reset(context.Context) error
	})
	if !ok {
		return fmt.Errorf("channel %q does not support reset operation", key)
	}

	return resettable.Reset(ctx)
}
