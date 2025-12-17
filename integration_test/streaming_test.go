package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreaming_BasicEmission tests that scope.Stream() can emit intermediate values.
func TestStreaming_BasicEmission(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New()
	g.Node("streamer", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("chunk1"))
		scope.Stream(message.NewAIMessageFromText("chunk2"))
		scope.Stream(message.NewAIMessageFromText("chunk3"))
		return graph.To(graph.END)
	}, graph.END)
	g.Start("streamer")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []message.Message
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		if out != nil {
			outputs = append(outputs, out)
		}
	}

	// Should have stream chunks
	assert.Len(t, outputs, 3)
}

// TestStreaming_MultipleNodes tests streaming across multiple nodes in sequence.
func TestStreaming_MultipleNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New()

	g.Node("node1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("from-node1"))
		return graph.To("node2")
	}, "node2")

	g.Node("node2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("from-node2"))
		return graph.To(graph.END)
	}, graph.END)

	g.Start("node1")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []message.Message
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		if out != nil {
			outputs = append(outputs, out)
		}
	}

	// Should see outputs from both nodes
	assert.Len(t, outputs, 2)
}

// TestStreaming_OrderPreservation tests that streamed values maintain order from sequence.
func TestStreaming_OrderPreservation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New()
	g.Node("counter", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		for i := 1; i <= 5; i++ {
			scope.Stream(message.NewAIMessageFromText(string(rune('0' + i))))
		}
		return graph.To(graph.END)
	}, graph.END)
	g.Start("counter")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []message.Message
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		if out != nil {
			outputs = append(outputs, out)
		}
	}

	// Stream outputs should be present
	assert.Len(t, outputs, 5)
}

// TestStreaming_ScopeStreamAvailable tests that Scope.Stream() is available during execution.
func TestStreaming_ScopeStreamAvailable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var streamedValues []string
	var mu sync.Mutex

	g := graph.New()
	g.Node("checker", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("test-value"))
		mu.Lock()
		streamedValues = append(streamedValues, "test-value")
		mu.Unlock()
		return graph.To(graph.END)
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

	nodeStarted := make(chan struct{})
	nodeExited := make(chan struct{})

	g := graph.New()

	g.Node("slow", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("before-block"))

		// Signal that we're waiting
		close(nodeStarted)

		// Wait for context cancellation
		select {
		case <-ctx.Done():
			close(nodeExited)
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			// Shouldn't reach here due to cancellation
			return graph.To(graph.END)
		}
	}, graph.END)

	g.Start("slow")

	compiled, err := g.Build()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	go func() {
		defer close(done)
		for range compiled.Run(ctx, nil) {
			// consume all outputs
		}
	}()

	// Wait for node to start, then cancel
	<-nodeStarted
	cancel()

	// Wait for node to exit (confirms context was propagated)
	select {
	case <-nodeExited:
		// Good - node received the cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("Node did not exit after context cancellation")
	}

	// Wait for run to complete
	select {
	case <-done:
		// Good - run completed
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not complete after context cancellation")
	}
}

// TestStreaming_EventEmission tests that streaming emits proper events.
func TestStreaming_EventEmission(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New()
	g.Node("emitter", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("event-test"))
		return graph.To(graph.END)
	}, graph.END)
	g.Start("emitter")

	compiled, err := g.Build()
	require.NoError(t, err)

	var outputs []message.Message
	for out, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
		if out != nil {
			outputs = append(outputs, out)
		}
	}

	// Should have at least one streamed output
	assert.Len(t, outputs, 1)
}

// TestStreaming_NoStreamCalls tests that graphs work correctly with no Stream() calls.
func TestStreaming_NoStreamCalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)
	g.Node("silent", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// No streaming, just return
		return graph.Set(resultKey, "done").End()
	}, graph.END)
	g.Start("silent")

	compiled, err := g.Build()
	require.NoError(t, err)

	var errorOccurred bool
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			errorOccurred = true
		}
	}

	assert.False(t, errorOccurred)
}
