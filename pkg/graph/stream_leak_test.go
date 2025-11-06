package graph

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraphStream_NoLeakOnEarlyTermination verifies that closing a stream
// before consuming all events doesn't leak goroutines.
func TestGraphStream_NoLeakOnEarlyTermination(t *testing.T) {
	t.Parallel()

	// Record initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Create a graph with multiple nodes
	builder := NewBuilder()
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		builder.Node(name, func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			time.Sleep(10 * time.Millisecond) // Simulate work
			return &NodeResult{
				Updates: map[string]any{"count": 1},
			}, nil
		})
		builder.AddEdge(StartNode, name)
	}
	compiled, err := builder.Compile()
	require.NoError(t, err)

	// Start streaming but only read one event
	stream, err := compiled.Stream(context.Background(), nil)
	require.NoError(t, err)

	// Read only first event
	if stream.Next() {
		_ = stream.Current()
	}

	// Close stream early (before all events consumed)
	err = stream.Close()
	assert.NoError(t, err)

	// Give time for goroutines to exit
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	// Check goroutine count - should be close to initial
	// Allow for some variance (±2 goroutines for runtime overhead)
	finalGoroutines := runtime.NumGoroutine()
	diff := finalGoroutines - initialGoroutines
	assert.LessOrEqual(t, diff, 2,
		"goroutine leak detected: started with %d, ended with %d (diff: %d)",
		initialGoroutines, finalGoroutines, diff)
}

// TestGraphStream_CloseIsIdempotent verifies Close can be called multiple times safely.
func TestGraphStream_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	builder.Node("test", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		return nil, nil
	})
	builder.StartTo("test").ToEnd("test")

	compiled, err := builder.Compile()
	require.NoError(t, err)

	stream, err := compiled.Stream(context.Background(), nil)
	require.NoError(t, err)

	// Close multiple times - should not panic
	assert.NotPanics(t, func() {
		_ = stream.Close()
		_ = stream.Close()
		_ = stream.Close()
	})
}

// TestGraphStream_CancelStopsExecution verifies that Cancel stops the graph execution.
func TestGraphStream_CancelStopsExecution(t *testing.T) {
	t.Parallel()

	var executionCount atomic.Int32
	builder := NewBuilder()

	// Create a chain of nodes
	for i := 0; i < 100; i++ {
		name := string(rune('a' + (i % 26)))
		if i > 0 {
			name = name + string(rune('0'+i/26))
		}
		builder.Node(name, func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			executionCount.Add(1)
			time.Sleep(5 * time.Millisecond)
			return nil, nil
		})
		if i == 0 {
			builder.AddEdge(StartNode, name)
		} else {
			prevName := string(rune('a' + ((i - 1) % 26)))
			if i > 1 {
				prevName = prevName + string(rune('0'+(i-1)/26))
			}
			builder.AddEdge(prevName, name)
		}
	}

	compiled, err := builder.Compile()
	require.NoError(t, err)

	stream, err := compiled.Stream(context.Background(), nil)
	require.NoError(t, err)

	// Read a few events then cancel
	eventCount := 0
	for stream.Next() && eventCount < 5 {
		eventCount++
	}

	stream.Cancel()

	// Execution should stop soon
	time.Sleep(50 * time.Millisecond)

	// Should not have executed all 100 nodes
	assert.Less(t, int(executionCount.Load()), 100, "execution should have been cancelled")
}
