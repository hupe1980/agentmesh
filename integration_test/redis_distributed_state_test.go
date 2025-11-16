package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestRedisMessageBus_GraphExecution tests graph execution with Redis message bus.
// This validates that distributed state synchronization works correctly with real graph nodes.
//
// NOTE: This test is skipped because it tests old architecture behavior that doesn't apply to Phase 2.
// In the old architecture, Redis distributed state updates across graph nodes.
// In Phase 2: Graph nodes share a single StateManager (no distribution), and Redis MessageBus
// is only for coordinating separate Pregel runtime instances (multi-process/multi-machine setups).
// For single-process graph execution, distributed state sync is unnecessary and causes issues.
func TestRedisMessageBus_GraphExecution(t *testing.T) {
	t.Skip("Test relies on old architecture's distributed state synchronization across graph nodes. Phase 2 uses shared StateManager within process.")

	ctx := context.Background()

	// Start Redis container
	container, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	// Create Redis message bus - use specialized MessageMessageBus for message.Message interface serialization
	bus := predis.NewMessageMessageBus(addr, "", 0, &predis.Options{
		Namespace: "test-graph-execution",
		TTL:       2 * time.Minute,
	})
	defer bus.Close()

	// Create graph state with a counter and history channel
	stateManager, err := state.NewChannelState(0)
	require.NoError(t, err)

	stateManager.Set("counter", 0)
	stateManager.AddChannel(channel.NewTopicChannel("history", 0))

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Add three sequential nodes
	require.NoError(t, g.AddNode(&graph.Node{
		Name: "node1",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			counter, _ := s.Get("counter").(int)
			return &graph.NodeResult{
				Updates: map[string]any{
					"counter": counter + 1,
					"history": []string{"node1"},
				},
			}, nil
		},
	}))

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "node2",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			counter, _ := s.Get("counter").(int)
			return &graph.NodeResult{
				Updates: map[string]any{
					"counter": counter + 1,
					"history": []string{"node2"},
				},
			}, nil
		},
	}))

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "node3",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			counter, _ := s.Get("counter").(int)
			return &graph.NodeResult{
				Updates: map[string]any{
					"counter": counter + 1,
					"history": []string{"node3"},
				},
			}, nil
		},
	}))

	// Build linear topology
	g.AddEdge(graph.StartNode, "node1")
	g.AddEdge("node1", "node2")
	g.AddEdge("node2", "node3")
	g.AddEdge("node3", graph.EndNode)

	// Compile graph with Pregel executor configured for distributed state
	// Note: WithMessageBus() automatically enables distributed state synchronization
	compiled, err := exec.CompileGraph(g,
		exec.WithExecutor(exec.NewPregelExecutor(
			exec.WithMessageBus(bus), // Automatically enables distributed state
		)),
	)
	require.NoError(t, err)

	// Execute with distributed state synchronization
	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Verify state updates
	counter := stateManager.Get("counter")
	require.Equal(t, 3, counter, "Counter should be 3")

	history := stateManager.Get("history")
	t.Logf("History type: %T, value: %v", history, history)

	// TopicChannel returns []any where each entry is the value we appended
	if historyAny, ok := history.([]any); ok {
		require.Len(t, historyAny, 3, "History should have 3 entries")
		// Flatten the history entries
		var flat []string
		for _, item := range historyAny {
			if slice, ok := item.([]string); ok {
				flat = append(flat, slice...)
			}
		}
		require.Equal(t, []string{"node1", "node2", "node3"}, flat)
	} else {
		t.Fatalf("History should be []any, got %T", history)
	}
}

// TestRedisMessageBus_ParallelNodes tests parallel node execution with distributed state.
func TestRedisMessageBus_ParallelNodes(t *testing.T) {
	t.Skip("Test relies on old architecture's distributed state synchronization across graph nodes. Phase 2 uses shared StateManager within process.")

	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	bus := predis.NewMessageMessageBus(addr, "", 0, &predis.Options{
		Namespace: "test-parallel",
	})
	defer bus.Close()

	stateManager, err := state.NewChannelState(0)
	require.NoError(t, err)

	stateManager.AddChannel(channel.NewTopicChannel("completed", 0))

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Create 3 parallel workers
	for i := 1; i <= 3; i++ {
		nodeNum := i
		require.NoError(t, g.AddNode(&graph.Node{
			Name: fmt.Sprintf("worker%d", nodeNum),
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				time.Sleep(10 * time.Millisecond)
				return &graph.NodeResult{
					Updates: map[string]any{
						"completed": []string{fmt.Sprintf("worker%d", nodeNum)},
					},
				}, nil
			},
		}))
	}

	// Add aggregator
	require.NoError(t, g.AddNode(&graph.Node{
		Name: "aggregator",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{}}, nil
		},
	}))

	// Build topology: all workers run in parallel, then aggregator
	for i := 1; i <= 3; i++ {
		g.AddEdge(graph.StartNode, fmt.Sprintf("worker%d", i))
		g.AddEdge(fmt.Sprintf("worker%d", i), "aggregator")
	}
	g.AddEdge("aggregator", graph.EndNode)

	compiled, err := exec.CompileGraph(g,
		exec.WithExecutor(exec.NewPregelExecutor(
			exec.WithMessageBus(bus), // Automatically enables distributed state
		)),
	)
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Verify all workers completed
	completed := stateManager.Get("completed")
	if completedAny, ok := completed.([]any); ok {
		require.Len(t, completedAny, 3, "All 3 workers should complete")
	} else {
		t.Fatalf("Completed should be []any, got %T", completed)
	}
}

// TestRedisMessageBus_ConditionalEdges tests conditional routing with distributed state.
func TestRedisMessageBus_ConditionalEdges(t *testing.T) {
	t.Skip("Test relies on old architecture's distributed state synchronization across graph nodes. Phase 2 uses shared StateManager within process.")

	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	bus := predis.NewMessageMessageBus(addr, "", 0, &predis.Options{
		Namespace: "test-conditional",
	})
	defer bus.Close()

	stateManager, err := state.NewChannelState(0)
	require.NoError(t, err)

	stateManager.Set("value", 42)
	stateManager.Set("path", "start")

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "start",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{}}, nil
		},
	}))

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "positive",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			path, _ := s.Get("path").(string)
			return &graph.NodeResult{
				Updates: map[string]any{
					"path": path + "->positive",
				},
			}, nil
		},
	}))

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "negative",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			path, _ := s.Get("path").(string)
			return &graph.NodeResult{
				Updates: map[string]any{
					"path": path + "->negative",
				},
			}, nil
		},
	}))

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "end",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			path, _ := s.Get("path").(string)
			return &graph.NodeResult{
				Updates: map[string]any{
					"path": path + "->end",
				},
			}, nil
		},
	}))

	// Build topology with conditional routing
	g.AddEdge(graph.StartNode, "start")
	g.AddConditionalEdges("start", func(ctx context.Context, s state.Reader) []string {
		value, _ := s.Get("value").(int)
		if value > 0 {
			return []string{"positive"}
		}
		return []string{"negative"}
	}, []string{"positive", "negative"})
	g.AddEdge("positive", "end")
	g.AddEdge("negative", "end")
	g.AddEdge("end", graph.EndNode)

	compiled, err := exec.CompileGraph(g,
		exec.WithExecutor(exec.NewPregelExecutor(
			exec.WithMessageBus(bus), // Automatically enables distributed state
		)),
	)
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Verify it took the positive path (value=42 > 0)
	path := stateManager.Get("path")
	require.Equal(t, "start->positive->end", path)
}
