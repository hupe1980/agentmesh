package event

import (
	"context"
	"sync"
	"sync/atomic"
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
// Thread-safe for concurrent publishers and subscribers. Handlers execute
// outside of internal locks so slow subscribers cannot block publishers.
type Bus struct {
	mu       sync.Mutex
	snapshot atomic.Pointer[handlerSnapshot]
}

type handlerSnapshot struct {
	handlers map[Type][]Handler
	all      []Handler
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	bus := &Bus{}
	bus.snapshot.Store(&handlerSnapshot{handlers: make(map[Type][]Handler)})
	return bus
}

// clone returns a shallow copy of the snapshot so modifications do not race
// with readers. Individual handler slices remain immutable until replaced.
func (s *handlerSnapshot) clone() *handlerSnapshot {
	if s == nil {
		return &handlerSnapshot{handlers: make(map[Type][]Handler)}
	}
	handlers := make(map[Type][]Handler, len(s.handlers))
	for t, list := range s.handlers {
		handlers[t] = list
	}
	return &handlerSnapshot{
		handlers: handlers,
		all:      s.all,
	}
}

func copyAndAppend(list []Handler, handler Handler) []Handler {
	cloned := append([]Handler(nil), list...)
	return append(cloned, handler)
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
	if handler == nil {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	current := eb.snapshot.Load()
	next := current.clone()

	if len(types) == 0 {
		next.all = copyAndAppend(next.all, handler)
		if next.handlers == nil {
			next.handlers = make(map[Type][]Handler)
		}
		// Store updated snapshot
		eb.snapshot.Store(next)
		return
	}

	if next.handlers == nil {
		next.handlers = make(map[Type][]Handler, len(types))
	}
	for _, t := range types {
		next.handlers[t] = copyAndAppend(next.handlers[t], handler)
	}
	eb.snapshot.Store(next)
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
	snapshot := eb.snapshot.Load()
	if snapshot == nil {
		return
	}

	if handlers := snapshot.handlers[event.Type]; len(handlers) > 0 {
		for _, h := range handlers {
			_ = h.HandleEvent(ctx, event) // Swallow errors
		}
	}

	for _, h := range snapshot.all {
		_ = h.HandleEvent(ctx, event)
	}
}
