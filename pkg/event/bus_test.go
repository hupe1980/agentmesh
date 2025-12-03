package event

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestBusPublishSpecificAndAllHandlers(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var specific atomic.Int32
	var all atomic.Int32

	bus.Subscribe(HandlerFunc(func(ctx context.Context, event Event) error {
		specific.Add(1)
		return nil
	}), EventNodeStart)

	bus.Subscribe(HandlerFunc(func(ctx context.Context, event Event) error {
		all.Add(1)
		return nil
	}))

	bus.Publish(ctx, Event{Type: EventNodeStart})

	if got := specific.Load(); got != 1 {
		t.Fatalf("expected specific handler to be called once, got %d", got)
	}
	if got := all.Load(); got != 1 {
		t.Fatalf("expected all handler to be called once, got %d", got)
	}
}

func TestBusSnapshotIsolation(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var first atomic.Int32
	bus.Subscribe(HandlerFunc(func(ctx context.Context, event Event) error {
		first.Add(1)
		return nil
	}), EventNodeStart)

	bus.Publish(ctx, Event{Type: EventNodeStart})

	var second atomic.Int32
	bus.Subscribe(HandlerFunc(func(ctx context.Context, event Event) error {
		second.Add(1)
		return nil
	}), EventNodeStart)

	bus.Publish(ctx, Event{Type: EventNodeStart})

	if got := first.Load(); got != 2 {
		t.Fatalf("expected first handler to be called twice, got %d", got)
	}
	if got := second.Load(); got != 1 {
		t.Fatalf("expected second handler to be called once, got %d", got)
	}
}
