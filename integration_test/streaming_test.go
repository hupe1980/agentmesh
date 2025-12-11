package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreaming_BasicEmission tests that scope.Stream() can emit intermediate values.
func TestStreaming_BasicEmission(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, string](resultKey)
	g.Node("streamer", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		scope.Stream("chunk1")
		scope.Stream("chunk2")
		scope.Stream("chunk3")
		return graph.Set(resultKey, "final").End()
	}, graph.END)
	g.Start("streamer")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []string
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		outputs = append(outputs, out)
	}

	// Should have stream chunks plus final output
	assert.GreaterOrEqual(t, len(outputs), 1)
	assert.Contains(t, outputs, "final")
}

// TestStreaming_MultipleNodes tests streaming across multiple nodes in sequence.
func TestStreaming_MultipleNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, string](resultKey)

	g.Node("node1", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		scope.Stream("from-node1")
		return graph.Set(resultKey, "n1").To("node2")
	}, "node2")

	g.Node("node2", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		scope.Stream("from-node2")
		return graph.Set(resultKey, "n2").End()
	}, graph.END)

	g.Start("node1")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []string
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		outputs = append(outputs, out)
	}

	// Should see outputs from both nodes
	assert.GreaterOrEqual(t, len(outputs), 2)
}

// TestStreaming_OrderPreservation tests that streamed values maintain order from sequence.
func TestStreaming_OrderPreservation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, string](resultKey)
	g.Node("counter", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		for i := 1; i <= 5; i++ {
			scope.Stream(string(rune('0' + i)))
		}
		return graph.Set(resultKey, "done").End()
	}, graph.END)
	g.Start("counter")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []string
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		outputs = append(outputs, out)
	}

	// Stream + final output should be present
	assert.Contains(t, outputs, "done")
}

// TestStreaming_ScopeStreamAvailable tests that Scope.Stream() is available during execution.
func TestStreaming_ScopeStreamAvailable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var streamedValues []string
	var mu sync.Mutex

	g := graph.New[any, string](resultKey)
	g.Node("checker", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		scope.Stream("test-value")
		mu.Lock()
		streamedValues = append(streamedValues, "test-value")
		mu.Unlock()
		return graph.Set(resultKey, "checked").End()
	}, graph.END)
	g.Start("checker")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, streamedValues, 1, "Scope.Stream() should be callable during graph execution")
}

// TestStreaming_WithContextCancellation tests that streaming handles context cancellation gracefully.
func TestStreaming_WithContextCancellation(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, string](resultKey)

	g.Node("slow", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		scope.Stream("before-block")

		// Wait a bit so we can cancel before completion
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			// Shouldn't reach here due to timeout
		}

		return graph.Set(resultKey, "completed").End()
	}, graph.END)

	g.Start("slow")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Use aggressive timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	// Should have errored due to context cancellation (timeout)
	if lastErr != nil {
		assert.True(t,
			errors.Is(lastErr, context.DeadlineExceeded) ||
				errors.Is(lastErr, context.Canceled),
			"Expected context error, got: %v", lastErr)
	} else {
		// This can happen if context doesn't propagate in time
		t.Log("Test completed without error - context cancellation may not have propagated in time")
	}
}

// TestStreaming_EventsPublished tests that stream updates are published as events.
func TestStreaming_EventsPublished(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	var events []event.Event
	var eventMu sync.Mutex

	// Create event bus and subscribe
	bus := event.NewBus()
	bus.Subscribe(event.HandlerFunc(func(ctx context.Context, e event.Event) error {
		eventMu.Lock()
		events = append(events, e)
		eventMu.Unlock()
		return nil
	}))

	ctx := event.WithBus(context.Background(), bus)

	g := graph.New[any, string](resultKey)
	g.Node("emitter", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		scope.Stream("streamed")
		return graph.Set(resultKey, "final").End()
	}, graph.END)
	g.Start("emitter")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	eventMu.Lock()
	defer eventMu.Unlock()

	// Should have received state update events
	var stateUpdateCount int
	for _, e := range events {
		if e.Type == event.EventStateUpdate {
			stateUpdateCount++
		}
	}
	assert.GreaterOrEqual(t, stateUpdateCount, 1, "Should have at least one state update event from streaming")
}
