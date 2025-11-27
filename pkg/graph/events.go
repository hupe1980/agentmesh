package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/event"
)

// Type aliases for backward compatibility
type (
	// EventType represents the type of execution event (backward compatibility alias).
	EventType = event.Type
	// Event represents an execution event (backward compatibility alias).
	Event = event.Event
	// EventHandler processes events (backward compatibility alias).
	EventHandler = event.Handler
	// EventBus manages event subscriptions and publishing (backward compatibility alias).
	EventBus = event.Bus
)

// Re-export event type constants
const (
	EventGraphStart        = event.EventGraphStart
	EventGraphComplete     = event.EventGraphComplete
	EventGraphError        = event.EventGraphError
	EventSuperstepStart    = event.EventSuperstepStart
	EventSuperstepComplete = event.EventSuperstepComplete
	EventNodeQueued        = event.EventNodeQueued
	EventNodeStart         = event.EventNodeStart
	EventNodeComplete      = event.EventNodeComplete
	EventNodeError         = event.EventNodeError
	EventStateUpdate       = event.EventStateUpdate
	EventCheckpointSave    = event.EventCheckpointSave
	EventCheckpointLoad    = event.EventCheckpointLoad
	EventCheckpointError   = event.EventCheckpointError
	EventInterrupt         = event.EventInterrupt
	EventResume            = event.EventResume
	EventModelStart        = event.EventModelStart
	EventModelComplete     = event.EventModelComplete
	EventModelError        = event.EventModelError
	EventToolStart         = event.EventToolStart
	EventToolComplete      = event.EventToolComplete
	EventToolError         = event.EventToolError
)

// EventHandlerFunc is a function adapter for EventHandler.
type EventHandlerFunc = event.HandlerFunc

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return event.NewBus()
}

// WithEventBus attaches an event bus to the context.
func WithEventBus(ctx context.Context, bus *EventBus) context.Context {
	return event.WithBus(ctx, bus)
}

// EventBusFromContext retrieves the event bus from context.
func EventBusFromContext(ctx context.Context) *EventBus {
	return event.BusFromContext(ctx)
}

// Publish is a helper that publishes an event if a bus exists in context.
func Publish(ctx context.Context, evt Event) {
	event.Publish(ctx, evt)
}
