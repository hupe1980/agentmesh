package graph

import (
	"context"
	"iter"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStream_NoLeakOnEarlyTermination verifies that closing a stream
// before consuming all events doesn't leak goroutines.
func TestStream_NoLeakOnEarlyTermination(t *testing.T) {
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
	ctx, cancel := context.WithCancel(context.Background())

	seq := compiled.Run(ctx, nil)
	pull, stop := iter.Pull2(seq)
	defer stop()

	// Read only first event
	_, _, ok := pull()
	if ok {
		// continue
	}

	// Cancel the context to stop the stream
	cancel()

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

// TestStream_CancelStopsExecution verifies that Cancel stops the graph execution.
func TestStream_CancelStopsExecution(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seq := compiled.Run(ctx, nil)
	pull, stop := iter.Pull2(seq)
	defer stop()

	// Read a few events then cancel
	eventCount := 0
	for eventCount < 5 {
		_, _, ok := pull()
		if !ok {
			break
		}
		eventCount++
	}

	cancel()

	// Give time for cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	// Get the final execution count
	finalCount := executionCount.Load()

	// The execution should stop soon after cancellation.
	// It should not execute all 100 nodes.
	assert.Less(t, finalCount, int32(100), "execution should have been cancelled early")
	assert.Greater(t, finalCount, int32(0), "at least some nodes should have executed")
}
