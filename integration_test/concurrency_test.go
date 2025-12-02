// Package integration_test contains integration tests for concurrent execution behavior.
package integration_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrency_ParallelNodeExecution tests that parallel nodes execute concurrently.
func TestConcurrency_ParallelNodeExecution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey("result", "")
	var concurrentCount atomic.Int32
	var maxConcurrent atomic.Int32

	g := graph.New[any, any](resultKey)

	// Start node triggers 3 parallel branches
	g.Node("start", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To("worker1", "worker2", "worker3")
	}, "worker1", "worker2", "worker3")

	// Create workers that track concurrent execution
	makeWorker := func(name string) graph.NodeFunc {
		return func(_ context.Context, _ graph.View) (*graph.Command, error) {
			current := concurrentCount.Add(1)
			// Track max concurrent
			for {
				old := maxConcurrent.Load()
				if current <= old || maxConcurrent.CompareAndSwap(old, current) {
					break
				}
			}

			time.Sleep(50 * time.Millisecond) // Simulate work

			concurrentCount.Add(-1)
			return graph.To("end")
		}
	}

	g.Node("worker1", makeWorker("worker1"), "end")
	g.Node("worker2", makeWorker("worker2"), "end")
	g.Node("worker3", makeWorker("worker3"), "end")

	g.Node("end", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.Set(resultKey, "done").End()
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Should have seen at least 2 concurrent workers (proving parallelism)
	assert.GreaterOrEqual(t, int(maxConcurrent.Load()), 2,
		"Expected at least 2 workers running concurrently")
}

// TestConcurrency_RaceConditionFree tests that state updates are thread-safe.
func TestConcurrency_RaceConditionFree(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	counterKey := graph.NewListKey[int]("counters")

	g := graph.New[any, any](counterKey)

	// Start triggers many parallel workers
	g.Node("start", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To("w1", "w2", "w3", "w4", "w5")
	}, "w1", "w2", "w3", "w4", "w5")

	// Each worker appends its ID
	for i, name := range []string{"w1", "w2", "w3", "w4", "w5"} {
		id := i + 1
		g.Node(name, func(_ context.Context, _ graph.View) (*graph.Command, error) {
			return graph.Append(counterKey, id).To("end")
		}, "end")
	}

	g.Node("end", func(_ context.Context, view graph.View) (*graph.Command, error) {
		counters := graph.GetList(view, counterKey)
		// Verify all 5 workers contributed
		if len(counters) != 5 {
			t.Errorf("Expected 5 items, got %d", len(counters))
		}
		return graph.To(graph.END)
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}

// TestConcurrency_ContextCancellation tests that concurrent nodes respect context cancellation.
func TestConcurrency_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	resultKey := graph.NewKey("result", "")
	var nodesStarted atomic.Int32

	g := graph.New[any, any](resultKey)

	g.Node("start", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To("slow1", "slow2")
	}, "slow1", "slow2")

	// Long-running nodes
	g.Node("slow1", func(ctx context.Context, _ graph.View) (*graph.Command, error) {
		nodesStarted.Add(1)
		select {
		case <-time.After(5 * time.Second):
			return graph.To("end")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, "end")

	g.Node("slow2", func(ctx context.Context, _ graph.View) (*graph.Command, error) {
		nodesStarted.Add(1)
		select {
		case <-time.After(5 * time.Second):
			return graph.To("end")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, "end")

	g.Node("end", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.Set(resultKey, "done").End()
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	for _, err := range compiled.Run(ctx, nil) {
		// We expect either an error or early termination
		_ = err
	}

	// Should finish quickly due to cancellation, not after 5 seconds
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 2*time.Second, "Should have cancelled quickly")

	// Verify that at least one slow node started before cancellation
	assert.GreaterOrEqual(t, int(nodesStarted.Load()), 1, "At least one slow node should have started")
}

// TestConcurrency_WaitGroupSemantics tests that merge nodes wait for all branches.
func TestConcurrency_WaitGroupSemantics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey("result", "")
	var completedBranches atomic.Int32

	g := graph.New[any, any](resultKey)

	g.Node("start", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To("fast", "slow")
	}, "fast", "slow")

	g.Node("fast", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		// Fast path completes immediately
		completedBranches.Add(1)
		return graph.To("merge")
	}, "merge")

	g.Node("slow", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		// Slow path takes some time
		time.Sleep(50 * time.Millisecond)
		completedBranches.Add(1)
		return graph.To("merge")
	}, "merge")

	g.Node("merge", func(_ context.Context, view graph.View) (*graph.Command, error) {
		// Both branches should have completed before this runs
		count := completedBranches.Load()
		if count != 2 {
			t.Errorf("Expected 2 completed branches, got %d", count)
		}
		return graph.Set(resultKey, "merged").End()
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(2), completedBranches.Load())
}

// TestConcurrency_HighFanout tests graph with high fan-out execution.
func TestConcurrency_HighFanout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewListKey[string]("results")
	numWorkers := 20

	g := graph.New[any, any](resultKey)

	// Build high fan-out graph
	workerNames := make([]string, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workerNames[i] = "worker" + string(rune('A'+i))
	}

	g.Node("start", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To(workerNames...)
	}, workerNames...)

	// Create all workers
	for _, name := range workerNames {
		workerName := name
		g.Node(workerName, func(_ context.Context, _ graph.View) (*graph.Command, error) {
			return graph.Append(resultKey, workerName).To("collect")
		}, "collect")
	}

	g.Node("collect", func(_ context.Context, view graph.View) (*graph.Command, error) {
		results := graph.GetList(view, resultKey)
		if len(results) != numWorkers {
			t.Errorf("Expected %d results, got %d", numWorkers, len(results))
		}
		return graph.To(graph.END)
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}

// TestConcurrency_MutexNotNeeded tests that internal state management is thread-safe.
func TestConcurrency_MutexNotNeeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Multiple iterations to stress test
	for iter := 0; iter < 10; iter++ {
		resultKey := graph.NewKey("result", 0)

		var wg sync.WaitGroup
		var completed atomic.Int32

		g := graph.New[any, any](resultKey)

		g.Node("start", func(_ context.Context, _ graph.View) (*graph.Command, error) {
			return graph.To("a", "b", "c", "d")
		}, "a", "b", "c", "d")

		for _, name := range []string{"a", "b", "c", "d"} {
			g.Node(name, func(_ context.Context, _ graph.View) (*graph.Command, error) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					completed.Add(1)
				}()
				return graph.To("end")
			}, "end")
		}

		g.Node("end", func(_ context.Context, _ graph.View) (*graph.Command, error) {
			return graph.Set(resultKey, int(completed.Load())).End()
		}, graph.END)

		g.Start("start")

		compiled, err := g.Build()
		require.NoError(t, err)

		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		wg.Wait()
		assert.Equal(t, int32(4), completed.Load())
	}
}
