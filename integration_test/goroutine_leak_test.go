package integration_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoGoroutineLeaks_SimpleGraph tests that simple graph execution doesn't leak goroutines.
func TestNoGoroutineLeaks_SimpleGraph(t *testing.T) {
	t.Parallel()

	// Force GC to clean up any pending goroutines
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	initialGoroutines := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		g := graph.New[string, string](ResultKey)

		g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			input := graph.Get(view, ResultKey)
			return graph.Set(ResultKey, input+"_processed").End()
		}, graph.END)

		g.Start("process")

		compiled, err := g.Build()
		require.NoError(t, err)

		for _, err := range compiled.Run(context.Background(), "test") {
			require.NoError(t, err)
		}
	}

	// Force GC and wait
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// Allow some tolerance for background goroutines
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Goroutine count increased significantly: before=%d, after=%d", initialGoroutines, finalGoroutines)
}

// TestNoGoroutineLeaks_CancelledContext tests cleanup after context cancellation.
func TestNoGoroutineLeaks_CancelledContext(t *testing.T) {
	t.Parallel()

	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	initialGoroutines := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithCancel(context.Background())

		g := graph.New[string, string](ResultKey)

		g.Node("slow", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			// Check for cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				// Continue
			}
			return graph.Set(ResultKey, "done").End()
		}, graph.END)

		g.Start("slow")

		compiled, err := g.Build()
		require.NoError(t, err)

		// Cancel immediately
		cancel()

		for _, err := range compiled.Run(ctx, "test") {
			// Expect context cancelled error
			_ = err
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Goroutine leak after cancellation: before=%d, after=%d", initialGoroutines, finalGoroutines)
}

// TestNoGoroutineLeaks_ParallelExecution tests cleanup in parallel execution scenarios.
func TestNoGoroutineLeaks_ParallelExecution(t *testing.T) {
	t.Parallel()

	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	initialGoroutines := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		g := graph.New[string, string](ResultKey)

		g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			return graph.Cmd().To("worker1", "worker2", "worker3")
		}, "worker1", "worker2", "worker3")

		for _, name := range []string{"worker1", "worker2", "worker3"} {
			workerName := name
			g.Node(workerName, func(ctx context.Context, view graph.View) (*graph.Command, error) {
				return graph.Cmd().To("merge")
			}, "merge")
		}

		g.Node("merge", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			return graph.Set(ResultKey, "merged").End()
		}, graph.END)

		g.Start("start")

		compiled, err := g.Build()
		require.NoError(t, err)

		for _, err := range compiled.Run(context.Background(), "test") {
			require.NoError(t, err)
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Goroutine leak in parallel execution: before=%d, after=%d", initialGoroutines, finalGoroutines)
}

// TestNoGoroutineLeaks_ErrorScenarios tests cleanup when nodes return errors.
func TestNoGoroutineLeaks_ErrorScenarios(t *testing.T) {
	t.Parallel()

	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	initialGoroutines := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		g := graph.New[string, string](ResultKey)

		g.Node("failing", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			return nil, assert.AnError
		}, graph.END)

		g.Start("failing")

		compiled, err := g.Build()
		require.NoError(t, err)

		for _, err := range compiled.Run(context.Background(), "test") {
			// Expect error
			assert.Error(t, err)
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Goroutine leak after errors: before=%d, after=%d", initialGoroutines, finalGoroutines)
}

// TestNoGoroutineLeaks_MultipleRuns tests cleanup across multiple runs of the same graph.
func TestNoGoroutineLeaks_MultipleRuns(t *testing.T) {
	t.Parallel()

	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	initialGoroutines := runtime.NumGoroutine()

	g := graph.New[string, string](ResultKey)

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		input := graph.Get(view, ResultKey)
		return graph.Set(ResultKey, input+"_done").End()
	}, graph.END)

	g.Start("process")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Run multiple times
	for i := 0; i < 20; i++ {
		for _, err := range compiled.Run(context.Background(), "test") {
			require.NoError(t, err)
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Goroutine leak after multiple runs: before=%d, after=%d", initialGoroutines, finalGoroutines)
}
