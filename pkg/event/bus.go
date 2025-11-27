package event

import (
	"context"
	"sync"
)

// Handler processes events published to the bus.
type Handler interface {
	// HandleEvent processes an event. Return error to signal handler failure.
	// Errors are logged but do not stop event propagation.
	HandleEvent(ctx context.Context, event Event) error
}

// HandlerFunc is a function adapter for Handler.
type HandlerFunc func(ctx context.Context, event Event) error

// HandleEvent implements the Handler interface.
func (f HandlerFunc) HandleEvent(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Bus provides pub/sub event distribution for graph execution.
// Thread-safe for concurrent publishers and subscribers.
type Bus struct {
	mu       sync.RWMutex
	handlers map[Type][]Handler
	all      []Handler // Handlers that receive all events
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[Type][]Handler),
		all:      []Handler{},
	}
}

// Subscribe registers a handler for specific event types.
// If no types are specified, the handler receives ALL events.
//
// Example:
//
//	// Subscribe to specific events
//	bus.Subscribe(handler, EventNodeStart, EventNodeComplete)
//
//	// Subscribe to all events
//	bus.Subscribe(handler)
func (eb *Bus) Subscribe(handler Handler, types ...Type) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if len(types) == 0 {
		// Subscribe to all events
		eb.all = append(eb.all, handler)
		return
	}

	// Subscribe to specific event types
	for _, t := range types {
		eb.handlers[t] = append(eb.handlers[t], handler)
	}
}

// Publish sends an event to all registered handlers.
// Handlers are called synchronously in the order they were subscribed.
// Handler errors are swallowed to prevent one handler from breaking others.
//
// Example:
//
//	bus.Publish(ctx, Event{
//	    Type: EventNodeStart,
//	    Node: "node1",
//	    Timestamp: time.Now(),
//	})
func (eb *Bus) Publish(ctx context.Context, event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Call type-specific handlers
	if handlers, ok := eb.handlers[event.Type]; ok {
		for _, h := range handlers {
			_ = h.HandleEvent(ctx, event) // Swallow errors
		}
	}

	// Call all-event handlers
	for _, h := range eb.all {
		_ = h.HandleEvent(ctx, event)
	}
}
