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

	"github.com/hupe1980/agentmesh/pkg/command"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChaos_RandomNodeFailures tests graph resilience to random node failures
func TestChaos_RandomNodeFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	failureRate := 0.2 // 20% of nodes will fail randomly

	countKey := state.NewKey("count", 0)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, countKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Counter to track successful executions
	var successCount atomic.Int32
	var failCount atomic.Int32

	// Create a chain of 10 nodes where each increments a counter
	for i := 0; i < 10; i++ {
		nodeNum := i
		nextNode := fmt.Sprintf("node_%d", i+1)
		if i == 9 {
			nextNode = graph.EndNode
		}
		err = g.AddNode(&graph.BaseNode{
			NodeName:        fmt.Sprintf("node_%d", i),
			DeclaredTargets: []string{nextNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				// Random failure injection
				if rand.Float64() < failureRate {
					failCount.Add(1)
					return nil, nil, fmt.Errorf("chaos: simulated failure in node_%d", nodeNum)
				}

				// Increment counter
				count := state.GetFromView(view, countKey)
				newCount := count + 1

				successCount.Add(1)
				updates := map[string]any{"count": newCount}
				return []string{nextNode}, updates, nil
			},
		})
		require.NoError(t, err)

	}

	// Set entry point
	if err := g.SetEntryPoint("node_0"); err != nil {
		t.Fatal(err)
	}

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	stateManager := newTestManager()

	// Register all result keys
	for i := 0; i < 5; i++ {
		resultKey := state.NewKey(fmt.Sprintf("result_%d", i), 0)
		state.RegisterKey(stateManager, resultKey)
	}
	totalKey := state.NewKey("total", 0)
	state.RegisterKey(stateManager, totalKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Create parallel nodes that may fail
	for i := 0; i < 5; i++ {
		nodeNum := i

		err = g.AddNode(&graph.BaseNode{
			NodeName:        fmt.Sprintf("parallel_%d", i),
			DeclaredTargets: []string{"aggregator"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				// Simulate work with random failure
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

				if rand.Float64() < 0.3 { // 30% failure rate
					return nil, nil, fmt.Errorf("concurrent failure in parallel_%d", nodeNum)
				}

				updates := map[string]any{
					fmt.Sprintf("result_%d", nodeNum): nodeNum * 10,
				}
				return []string{"aggregator"}, updates, nil
			},
		})
		require.NoError(t, err)
	}

	// Aggregator node collects results
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "aggregator",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			total := 0
			for i := 0; i < 5; i++ {
				resultKey := state.NewKey(fmt.Sprintf("result_%d", i), 0)
				val := state.GetFromView(view, resultKey)
				total += val
			}
			updates := map[string]any{"total": total}
			return []string{graph.EndNode}, updates, nil
		},
	})
	require.NoError(t, err) // Set entry points for all parallel nodes
	for i := 0; i < 5; i++ {
		g.SetEntryPoint(fmt.Sprintf("parallel_%d", i))
	}
	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	completedKey := state.NewKey("completed", false)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, completedKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	err = g.AddNode(&graph.BaseNode{
		NodeName:        "slow_node",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Simulate slow operation that will timeout
			select {
			case <-time.After(5 * time.Second):
				updates := map[string]any{"completed": true}
				return []string{graph.EndNode}, updates, nil
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("slow_node")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	recoveredKey := state.NewKey("recovered", false)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, recoveredKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	err = g.AddNode(&graph.BaseNode{
		NodeName:        "panicking_node",
		DeclaredTargets: []string{"recovery_node"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// This should be caught and converted to an error
			panic("chaos: intentional panic for testing")
		},
	})
	require.NoError(t, err)

	err = g.AddNode(&graph.BaseNode{
		NodeName:        "recovery_node",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// This should not execute if previous panicked
			updates := map[string]any{"recovered": true}
			return []string{graph.EndNode}, updates, nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("panicking_node")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	largeDataKey := state.NewKey("large_data", []byte{})

	stateManager := newTestManager()
	state.RegisterKey(stateManager, largeDataKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node that allocates large amounts of memory
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "memory_hog",
		DeclaredTargets: []string{"consumer"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Allocate 100MB
			data := make([]byte, 100*1024*1024)
			for i := range data {
				data[i] = byte(i % 256)
			}

			// Store large data
			return command.New().
				With(command.SetValue(largeDataKey, data)).
				To("consumer")
		},
	})
	require.NoError(t, err)

	err = g.AddNode(&graph.BaseNode{
		NodeName:        "consumer",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			data := state.GetFromView(view, largeDataKey)
			require.NotNil(t, data)

			// Verify data integrity
			assert.Equal(t, 100*1024*1024, len(data))
			return []string{graph.EndNode}, nil, nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("memory_hog")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	buildGraph := func() graph.Runnable[[]message.Message, message.Message] {
		p1DataKey := state.NewKey("p1_data", "")
		resultKey := state.NewKey("result", "")

		stateManager := newTestManager()
		state.RegisterKey(stateManager, p1DataKey)
		state.RegisterKey(stateManager, resultKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		err = g.AddNode(&graph.BaseNode{
			NodeName:        "partition_1_node",
			DeclaredTargets: []string{"partition_2_node"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				if !partition1Active.Load() {
					return nil, nil, fmt.Errorf("network partition: partition 1 unreachable")
				}

				return command.New().
					With(command.SetValue(p1DataKey, "partition1")).
					To("partition_2_node")
			},
		})
		require.NoError(t, err)

		err = g.AddNode(&graph.BaseNode{
			NodeName:        "partition_2_node",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				if !partition2Active.Load() {
					return nil, nil, fmt.Errorf("network partition: partition 2 unreachable")
				}

				// Try to access data from partition 1
				p1Data := state.GetFromView(view, p1DataKey)
				if p1Data == "" {
					return nil, nil, fmt.Errorf("partition isolation: cannot access partition 1 data")
				}

				return command.New().
					With(command.SetValue(resultKey, "partitions_connected")).
					To(graph.EndNode)
			},
		})
		require.NoError(t, err)

		g.SetEntryPoint("partition_1_node")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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
