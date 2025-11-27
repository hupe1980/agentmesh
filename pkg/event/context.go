package event

import (
	"context"
	"time"
)

type eventBusKey struct{}

// WithBus attaches an event bus to the context.
func WithBus(ctx context.Context, bus *Bus) context.Context {
	return context.WithValue(ctx, eventBusKey{}, bus)
}

// BusFromContext retrieves the event bus from context.
// Returns nil if no event bus is attached.
func BusFromContext(ctx context.Context) *Bus {
	if bus, ok := ctx.Value(eventBusKey{}).(*Bus); ok {
		return bus
	}
	return nil
}

// Publish publishes an event using the event bus from context.
// If no event bus is attached, this is a no-op.
// This is the recommended way to publish events.
func Publish(ctx context.Context, event Event) {
	if bus := BusFromContext(ctx); bus != nil {
		// Set timestamp if not already set
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now()
		}
		bus.Publish(ctx, event)
	}
}
