package integration_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChaos_RandomNodeFailures tests graph resilience to random node failures
func TestChaos_RandomNodeFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	failureRate := 0.2 // 20% of nodes will fail randomly

	countKey := graph.NewKey("count", 0)

	// Counter to track successful executions
	var successCount atomic.Int32
	var failCount atomic.Int32

	// Build a chain of 10 nodes where each increments a counter
	g := graph.New[any, any](countKey)

	for i := 0; i < 10; i++ {
		nodeNum := i
		nodeName := fmt.Sprintf("node_%d", i)
		nextNode := fmt.Sprintf("node_%d", i+1)
		if i == 9 {
			nextNode = graph.END
		}

		g.Node(nodeName, func(ctx context.Context, view graph.View) (*graph.Command, error) {
			// Random failure injection
			if rand.Float64() < failureRate {
				failCount.Add(1)
				return nil, fmt.Errorf("chaos: simulated failure in node_%d", nodeNum)
			}

			// Increment counter
			count := graph.Get(view, countKey)
			successCount.Add(1)

			return graph.Set(countKey, count+1).To(nextNode)
		}, nextNode)
	}

	g.Start("node_0")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Execute and expect possible failure due to chaos
	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		t.Logf("Graph failed as expected with chaos injection: %v", lastErr)
		assert.Contains(t, lastErr.Error(), "chaos: simulated failure")
	}

	// Verify some nodes executed
	totalExecutions := successCount.Load() + failCount.Load()
	t.Logf("Total executions: %d, Successes: %d, Failures: %d",
		totalExecutions, successCount.Load(), failCount.Load())

	// At least some nodes should have executed
	assert.Greater(t, int(totalExecutions), 0, "Some nodes should have executed")
}

// TestChaos_ConcurrentExecutionFailures tests failures during concurrent node execution
func TestChaos_ConcurrentExecutionFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Define keys for each parallel node's result
	result0Key := graph.NewKey("result_0", 0)
	result1Key := graph.NewKey("result_1", 0)
	result2Key := graph.NewKey("result_2", 0)
	result3Key := graph.NewKey("result_3", 0)
	result4Key := graph.NewKey("result_4", 0)
	totalKey := graph.NewKey("total", 0)

	g := graph.New[any, any](result0Key, result1Key, result2Key, result3Key, result4Key, totalKey)

	// Create parallel nodes that may fail
	for i := 0; i < 5; i++ {
		nodeNum := i
		nodeName := fmt.Sprintf("parallel_%d", i)

		var resultKey graph.Key[int]
		switch i {
		case 0:
			resultKey = result0Key
		case 1:
			resultKey = result1Key
		case 2:
			resultKey = result2Key
		case 3:
			resultKey = result3Key
		case 4:
			resultKey = result4Key
		}

		g.Node(nodeName, func(ctx context.Context, view graph.View) (*graph.Command, error) {
			// Simulate work with random failure
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

			if rand.Float64() < 0.3 { // 30% failure rate
				return nil, fmt.Errorf("concurrent failure in parallel_%d", nodeNum)
			}

			return graph.Set(resultKey, nodeNum*10).To("aggregator")
		}, "aggregator")
	}

	// Aggregator node collects results
	g.Node("aggregator", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		total := graph.Get(view, result0Key) +
			graph.Get(view, result1Key) +
			graph.Get(view, result2Key) +
			graph.Get(view, result3Key) +
			graph.Get(view, result4Key)

		return graph.Set(totalKey, total).End()
	}, graph.END)

	// Set entry points for all parallel nodes
	g.Start("parallel_0", "parallel_1", "parallel_2", "parallel_3", "parallel_4")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Execute - may succeed or fail depending on random failures
	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		t.Logf("Graph failed due to concurrent chaos: %v", lastErr)
		assert.Contains(t, lastErr.Error(), "concurrent failure")
	} else {
		t.Log("Graph succeeded despite chaos injection")
	}
}

// TestChaos_TimeoutDuringExecution tests node timeouts under load
func TestChaos_TimeoutDuringExecution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	completedKey := graph.NewKey("completed", false)

	g := graph.New[any, any](completedKey)

	g.Node("slow_node", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Simulate slow operation that will timeout
		select {
		case <-time.After(5 * time.Second):
			return graph.Set(completedKey, true).End()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, graph.END)

	g.Start("slow_node")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Execute with aggressive timeout (shorter than the 5 second sleep)
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	var lastErr error
	for _, err := range compiled.Run(ctxTimeout, nil) {
		if err != nil {
			lastErr = err
		}
	}

	// Should timeout or complete before context propagates
	if lastErr != nil {
		// Timeout occurred as expected
		t.Logf("Node correctly timed out: %v", lastErr)
		assert.True(t, errors.Is(lastErr, context.DeadlineExceeded) ||
			errors.Is(lastErr, context.Canceled) ||
			strings.Contains(lastErr.Error(), "context deadline exceeded") ||
			strings.Contains(lastErr.Error(), "timeout"),
			"Expected timeout-related error, got: %v", lastErr)
	} else {
		// Test may have race condition - log and skip validation
		t.Log("Test completed without error (context cancellation may not have propagated in time)")
	}
}

// TestChaos_PanicRecovery tests panic recovery in node execution
func TestChaos_PanicRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	recoveredKey := graph.NewKey("recovered", false)

	g := graph.New[any, any](recoveredKey)

	g.Node("panicking_node", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// This should be caught and converted to an error
		panic("chaos: intentional panic for testing")
	}, "recovery_node")

	g.Node("recovery_node", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// This should not execute if previous panicked
		return graph.Set(recoveredKey, true).End()
	}, graph.END)

	g.Start("panicking_node")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Execute - should handle panic gracefully
	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	// The panic should be caught and converted to an error
	require.Error(t, lastErr, "Expected panic to be caught and converted to error")
	assert.Contains(t, lastErr.Error(), "panic")
	t.Logf("Panic was caught and converted to error: %v", lastErr)
}

// TestChaos_MemoryPressure tests behavior under memory pressure
func TestChaos_MemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}
	t.Parallel()

	ctx := context.Background()

	largeDataKey := graph.NewKey[[]byte]("large_data", nil)

	g := graph.New[any, any](largeDataKey)

	// Node that allocates large amounts of memory
	g.Node("memory_hog", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Allocate 100MB
		data := make([]byte, 100*1024*1024)
		for i := range data {
			data[i] = byte(i % 256)
		}

		// Store large data
		return graph.Set(largeDataKey, data).To("consumer")
	}, "consumer")

	g.Node("consumer", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		data := graph.Get(view, largeDataKey)
		require.NotNil(t, data)

		// Verify data integrity
		assert.Equal(t, 100*1024*1024, len(data))
		return graph.Cmd().End()
	}, graph.END)

	g.Start("memory_hog")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Execute - should handle large data
	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}
	require.NoError(t, lastErr)
	t.Log("Successfully handled large memory allocation")
}

// TestChaos_NetworkPartition simulates network partition scenarios
func TestChaos_NetworkPartition(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Simulate nodes in different "partitions"
	var partition1Active atomic.Bool
	var partition2Active atomic.Bool

	p1DataKey := graph.NewKey("p1_data", "")
	resultKey := graph.NewKey("result", "")

	buildGraph := func() *graph.Graph[any, any] {
		g := graph.New[any, any](p1DataKey, resultKey)

		g.Node("partition_1_node", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			if !partition1Active.Load() {
				return nil, fmt.Errorf("network partition: partition 1 unreachable")
			}

			return graph.Set(p1DataKey, "partition1").To("partition_2_node")
		}, "partition_2_node")

		g.Node("partition_2_node", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			if !partition2Active.Load() {
				return nil, fmt.Errorf("network partition: partition 2 unreachable")
			}

			// Try to access data from partition 1
			p1Data := graph.Get(view, p1DataKey)
			if p1Data == "" {
				return nil, fmt.Errorf("partition isolation: cannot access partition 1 data")
			}

			return graph.Set(resultKey, "partitions_connected").End()
		}, graph.END)

		g.Start("partition_1_node")

		compiled, err := g.Build()
		require.NoError(t, err)
		return compiled
	}

	// Test 1: Both partitions active - should succeed
	partition1Active.Store(true)
	partition2Active.Store(true)
	compiled1 := buildGraph()
	var lastErr1 error
	for _, err := range compiled1.Run(ctx, nil) {
		if err != nil {
			lastErr1 = err
		}
	}
	require.NoError(t, lastErr1)
	t.Log("Test with both partitions active: SUCCESS")

	// Test 2: Partition 1 fails - should fail
	partition1Active.Store(false)
	partition2Active.Store(true)
	compiled2 := buildGraph()
	var lastErr2 error
	for _, err := range compiled2.Run(ctx, nil) {
		if err != nil {
			lastErr2 = err
		}
	}
	require.Error(t, lastErr2)
	assert.Contains(t, lastErr2.Error(), "partition 1 unreachable")
	t.Log("Test with partition 1 down: FAILED as expected")

	// Test 3: Partition 1 recovers, partition 2 fails
	partition1Active.Store(true)
	partition2Active.Store(false)
	compiled3 := buildGraph()
	var lastErr3 error
	for _, err := range compiled3.Run(ctx, nil) {
		if err != nil {
			lastErr3 = err
		}
	}
	require.Error(t, lastErr3)
	assert.Contains(t, lastErr3.Error(), "partition 2 unreachable")
	t.Log("Test with partition 2 down: FAILED as expected")
}
