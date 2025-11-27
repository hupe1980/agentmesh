package graph_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEventHandler is a test implementation that records events
type mockEventHandler struct {
	mu     sync.Mutex
	events []graph.Event
	errors []error
}

func (m *mockEventHandler) HandleEvent(ctx context.Context, event graph.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		return err
	}
	return nil
}

func (m *mockEventHandler) getEvents() []graph.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]graph.Event{}, m.events...)
}

func (m *mockEventHandler) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestEventHandlerFunc(t *testing.T) {
	var called bool
	var capturedEvent graph.Event

	handler := graph.EventHandlerFunc(func(ctx context.Context, event graph.Event) error {
		called = true
		capturedEvent = event
		return nil
	})

	event := graph.Event{
		Type:      graph.EventNodeStart,
		Node:      "test_node",
		Timestamp: time.Now(),
	}

	err := handler.HandleEvent(context.Background(), event)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, event.Type, capturedEvent.Type)
	assert.Equal(t, event.Node, capturedEvent.Node)
}

func TestEventHandlerFunc_Error(t *testing.T) {
	expectedErr := errors.New("handler error")

	handler := graph.EventHandlerFunc(func(ctx context.Context, event graph.Event) error {
		return expectedErr
	})

	err := handler.HandleEvent(context.Background(), graph.Event{})
	assert.Equal(t, expectedErr, err)
}

func TestNewEventBus(t *testing.T) {
	bus := graph.NewEventBus()
	assert.NotNil(t, bus)
}

func TestEventBus_Subscribe_SpecificType(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{}

	bus.Subscribe(handler, graph.EventNodeStart)

	// Publish matching event
	bus.Publish(context.Background(), graph.Event{
		Type: graph.EventNodeStart,
		Node: "node1",
	})

	events := handler.getEvents()
	require.Len(t, events, 1)
	assert.Equal(t, graph.EventNodeStart, events[0].Type)
}

func TestEventBus_Subscribe_MultipleTypes(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{}

	bus.Subscribe(handler, graph.EventNodeStart, graph.EventNodeComplete, graph.EventNodeError)

	// Publish matching events
	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeStart})
	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeComplete})
	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeError})

	// Publish non-matching event
	bus.Publish(context.Background(), graph.Event{Type: graph.EventGraphStart})

	events := handler.getEvents()
	assert.Equal(t, 3, len(events))
}

func TestEventBus_Subscribe_AllEvents(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{}

	// Subscribe without specifying types = receive all events
	bus.Subscribe(handler)

	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeStart})
	bus.Publish(context.Background(), graph.Event{Type: graph.EventGraphComplete})
	bus.Publish(context.Background(), graph.Event{Type: graph.EventStateUpdate})

	events := handler.getEvents()
	assert.Equal(t, 3, len(events))
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	bus := graph.NewEventBus()
	handler1 := &mockEventHandler{}
	handler2 := &mockEventHandler{}

	bus.Subscribe(handler1, graph.EventNodeStart)
	bus.Subscribe(handler2, graph.EventNodeStart)

	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeStart})

	assert.Equal(t, 1, handler1.count())
	assert.Equal(t, 1, handler2.count())
}

func TestEventBus_Publish_ErrorHandling(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{
		errors: []error{errors.New("handler error")},
	}

	bus.Subscribe(handler, graph.EventNodeStart)

	// Publish should not panic even if handler returns error
	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeStart})

	// Handler was still called
	assert.Equal(t, 1, handler.count())
}

func TestEventBus_Concurrent(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{}
	bus.Subscribe(handler)

	// Concurrent publishes
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bus.Publish(context.Background(), graph.Event{
				Type: graph.EventNodeStart,
				Data: map[string]any{"n": n},
			})
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 100, handler.count())
}

func TestEventBus_MixedSubscriptions(t *testing.T) {
	bus := graph.NewEventBus()

	specificHandler := &mockEventHandler{}
	allHandler := &mockEventHandler{}

	bus.Subscribe(specificHandler, graph.EventNodeStart)
	bus.Subscribe(allHandler) // All events

	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeStart})
	bus.Publish(context.Background(), graph.Event{Type: graph.EventNodeComplete})

	// Specific handler only gets NodeStart
	assert.Equal(t, 1, specificHandler.count())

	// All handler gets both events
	assert.Equal(t, 2, allHandler.count())
}

func TestWithEventBus(t *testing.T) {
	bus := graph.NewEventBus()
	ctx := context.Background()

	ctx = graph.WithEventBus(ctx, bus)
	assert.NotNil(t, ctx)

	retrievedBus := graph.EventBusFromContext(ctx)
	assert.Equal(t, bus, retrievedBus)
}

func TestEventBusFromContext_NoBus(t *testing.T) {
	ctx := context.Background()
	bus := graph.EventBusFromContext(ctx)
	assert.Nil(t, bus)
}

func TestPublish_WithBus(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{}
	bus.Subscribe(handler)

	ctx := graph.WithEventBus(context.Background(), bus)

	graph.Publish(ctx, graph.Event{
		Type: graph.EventNodeStart,
		Node: "test",
	})

	assert.Equal(t, 1, handler.count())
}

func TestPublish_NoBus(t *testing.T) {
	ctx := context.Background()

	// Should not panic when no bus in context
	graph.Publish(ctx, graph.Event{Type: graph.EventNodeStart})
}

func TestEvent_AllFields(t *testing.T) {
	now := time.Now()

	event := graph.Event{
		Type:      graph.EventNodeComplete,
		Timestamp: now,
		RunID:     "run-123",
		Superstep: 5,
		Node:      "processor",
		Data:      map[string]any{"key": "value"},
		Error:     "error message",
		Duration:  100 * time.Millisecond,
	}

	assert.Equal(t, graph.EventNodeComplete, event.Type)
	assert.Equal(t, now, event.Timestamp)
	assert.Equal(t, "run-123", event.RunID)
	assert.Equal(t, 5, event.Superstep)
	assert.Equal(t, "processor", event.Node)
	assert.Equal(t, "value", event.Data["key"])
	assert.Equal(t, "error message", event.Error)
	assert.Equal(t, 100*time.Millisecond, event.Duration)
}

func TestEventType_Constants(t *testing.T) {
	// Verify all event type constants are defined
	eventTypes := []graph.EventType{
		graph.EventGraphStart,
		graph.EventGraphComplete,
		graph.EventGraphError,
		graph.EventSuperstepStart,
		graph.EventSuperstepComplete,
		graph.EventNodeQueued,
		graph.EventNodeStart,
		graph.EventNodeComplete,
		graph.EventNodeError,
		graph.EventStateUpdate,
		graph.EventCheckpointSave,
		graph.EventCheckpointLoad,
		graph.EventCheckpointError,
		graph.EventInterrupt,
		graph.EventResume,
		graph.EventModelStart,
		graph.EventModelComplete,
		graph.EventModelError,
		graph.EventToolStart,
		graph.EventToolComplete,
		graph.EventToolError,
	}

	assert.Len(t, eventTypes, 21)

	// Verify they're all unique strings
	seen := make(map[graph.EventType]bool)
	for _, et := range eventTypes {
		assert.False(t, seen[et], "duplicate event type: %s", et)
		seen[et] = true
	}
}

func TestEventBus_OrderPreservation(t *testing.T) {
	bus := graph.NewEventBus()
	handler := &mockEventHandler{}
	bus.Subscribe(handler)

	// Publish events in order
	for i := 0; i < 10; i++ {
		bus.Publish(context.Background(), graph.Event{
			Type: graph.EventNodeStart,
			Data: map[string]any{"order": i},
		})
	}

	events := handler.getEvents()
	require.Len(t, events, 10)

	// Verify order is preserved
	for i, event := range events {
		assert.Equal(t, i, event.Data["order"])
	}
}

func TestEventBus_ContextPropagation(t *testing.T) {
	bus := graph.NewEventBus()

	type ctxKey string
	const testKey ctxKey = "test"

	var capturedValue string
	handler := graph.EventHandlerFunc(func(ctx context.Context, event graph.Event) error {
		if val := ctx.Value(testKey); val != nil {
			capturedValue = val.(string)
		}
		return nil
	})

	bus.Subscribe(handler)

	ctx := context.WithValue(context.Background(), testKey, "test-value")
	bus.Publish(ctx, graph.Event{Type: graph.EventNodeStart})

	assert.Equal(t, "test-value", capturedValue)
}
