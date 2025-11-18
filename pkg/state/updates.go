package state

import (
	"context"
	"fmt"
)

// Reset resets a channel to its initial empty state.
// This is used to clear list keys (TopicChannels) or reset LastValue channels.
// WARNING: This is a destructive operation. Use with caution.
func Reset(ctx context.Context, m *Manager, key string) error {
	ch := m.GetChannel(key)
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
