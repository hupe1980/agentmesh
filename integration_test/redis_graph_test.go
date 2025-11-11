package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/graph"
	predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestRedisMessageBus_GraphExecution tests graph execution with Redis message bus.
// This validates that the distributed message bus works correctly with real graph nodes.
func TestRedisMessageBus_GraphExecution(t *testing.T) {
	ctx := context.Background()

	// Start Redis container
	container, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	// Create Redis message bus
	bus := predis.NewMessageBus[graph.ChannelMessage](addr, "", 0, &predis.Options{
		Namespace: "test-graph-execution",
		TTL:       2 * time.Minute,
	})
	defer bus.Close()

	// Create graph state with a counter and history channel
	state := graph.NewState(0)
	state.Set("counter", 0)
	state.AddChannel(channel.NewTopicChannel("history", 0))

	g := graph.NewGraph(state)

	// Add three sequential nodes
	require.NoError(t, g.AddNode(&graph.Node{
		Name: "node1",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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

	// Compile graph
	compiled, err := g.Compile()
	require.NoError(t, err)

	// Execute with Redis message bus
	_, err = graph.Last(compiled.Run(ctx, nil, graph.WithPregelMessageBus(bus)))
	require.NoError(t, err)

	// Verify state updates
	counter := state.Get("counter")
	require.Equal(t, 3, counter, "Counter should be 3")

	history := state.Get("history")
	t.Logf("History type: %T, value: %v", history, history)

	// TopicChannel returns []any where each entry is the value we appended
	// Since we appended []string{"node1"}, etc., we get [["node1"], ["node2"], ["node3"]]
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

// TestRedisMessageBus_ParallelNodes tests parallel node execution with Redis.
func TestRedisMessageBus_ParallelNodes(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	bus := predis.NewMessageBus[graph.ChannelMessage](addr, "", 0, &predis.Options{
		Namespace: "test-parallel",
	})
	defer bus.Close()

	state := graph.NewState(0)
	state.AddChannel(channel.NewTopicChannel("completed", 0))

	g := graph.NewGraph(state)

	// Create 3 parallel workers
	for i := 1; i <= 3; i++ {
		nodeNum := i
		require.NoError(t, g.AddNode(&graph.Node{
			Name: fmt.Sprintf("worker%d", nodeNum),
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{}}, nil
		},
	}))

	// Build topology: all workers run in parallel, then aggregator
	for i := 1; i <= 3; i++ {
		g.AddEdge(graph.StartNode, fmt.Sprintf("worker%d", i))
		g.AddEdge(fmt.Sprintf("worker%d", i), "aggregator")
	}
	g.AddEdge("aggregator", graph.EndNode)

	compiled, err := g.Compile()
	require.NoError(t, err)

	_, err = graph.Last(compiled.Run(ctx, nil, graph.WithPregelMessageBus(bus)))
	require.NoError(t, err)

	// Verify all workers completed
	completed := state.Get("completed")
	if completedAny, ok := completed.([]any); ok {
		require.Len(t, completedAny, 3, "All 3 workers should complete")
	} else {
		t.Fatalf("Completed should be []any, got %T", completed)
	}
}

// TestRedisMessageBus_ConditionalEdges tests conditional routing with Redis backend.
func TestRedisMessageBus_ConditionalEdges(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	bus := predis.NewMessageBus[graph.ChannelMessage](addr, "", 0, &predis.Options{
		Namespace: "test-conditional",
	})
	defer bus.Close()

	state := graph.NewState(0)
	state.Set("value", 42)
	state.Set("path", "start")

	g := graph.NewGraph(state)

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "start",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{}}, nil
		},
	}))

	require.NoError(t, g.AddNode(&graph.Node{
		Name: "positive",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
	g.AddConditionalEdges("start", func(ctx context.Context, s graph.StateReader) []string {
		value, _ := s.Get("value").(int)
		if value > 0 {
			return []string{"positive"}
		}
		return []string{"negative"}
	}, []string{"positive", "negative"})
	g.AddEdge("positive", "end")
	g.AddEdge("negative", "end")
	g.AddEdge("end", graph.EndNode)

	compiled, err := g.Compile()
	require.NoError(t, err)

	_, err = graph.Last(compiled.Run(ctx, nil, graph.WithPregelMessageBus(bus)))
	require.NoError(t, err)

	// Verify it took the positive path (value=42 > 0)
	path := state.Get("path")
	require.Equal(t, "start->positive->end", path)
}
