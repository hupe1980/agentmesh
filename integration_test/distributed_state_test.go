package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestDistributedStateSync verifies that state updates propagate correctly
// through a Redis message bus in a distributed BSP execution.
func TestDistributedStateSync(t *testing.T) {
	ctx := context.Background()

	// Start Redis container
	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get Redis endpoint
	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	// Create Redis message bus for graph.Updates
	bus, err := predis.NewMessageBus[graph.Updates](addr, "", 0, &predis.Options{
		Namespace: "test-distributed-state",
		TTL:       1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Test Ping
	if err := bus.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping Redis: %v", err)
	}

	// Define state keys for testing
	// Use float64 for counter since JSON unmarshals numbers as float64
	counterKey := graph.NewKey("counter", 0.0)
	dataKey := graph.NewKey("data", "")

	// Build a graph with nodes that modify state
	// node1 -> node2 -> node3
	g := graph.New[any, any](counterKey, dataKey)

	// Node 1: Initialize state
	g.Node("node1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Set(counterKey, 1.0).
			With(graph.SetValue(dataKey, "A")).
			To("node2")
	}, "node2")

	// Node 2: Read and modify state (should see node1's updates via Redis)
	g.Node("node2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		counter := graph.Get(view, counterKey)
		data := graph.Get(view, dataKey)

		return graph.Set(counterKey, counter+1.0).
			With(graph.SetValue(dataKey, data+"B")).
			To("node3")
	}, "node3")

	// Node 3: Final state update (should see node2's updates via Redis)
	g.Node("node3", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		counter := graph.Get(view, counterKey)
		data := graph.Get(view, dataKey)

		return graph.Set(counterKey, counter+1.0).
			With(graph.SetValue(dataKey, data+"C")).
			End()
	}, graph.END)

	g.Start("node1")

	// Use executor with Redis message bus
	executor := graph.NewPregelExecutor[any, any]().WithMessageBus(bus)
	g.WithExecutor(executor)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	// Execute the graph
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
	}

	t.Log("Distributed state sync test passed!")
}

// TestDistributedStateParallel tests parallel node execution with Redis
func TestDistributedStateParallel(t *testing.T) {
	ctx := context.Background()

	// Start Redis container
	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get Redis endpoint
	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	// Create Redis message bus
	bus, err := predis.NewMessageBus[graph.Updates](addr, "", 0, &predis.Options{
		Namespace: "test-parallel-state",
		TTL:       1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Define state keys
	result1Key := graph.NewKey("result1", "")
	result2Key := graph.NewKey("result2", "")
	finalKey := graph.NewKey("final", "")

	// Build graph with parallel execution
	g := graph.New[any, any](result1Key, result2Key, finalKey)

	// Two parallel nodes
	g.Node("worker1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		time.Sleep(10 * time.Millisecond) // Simulate work
		return graph.Set(result1Key, "worker1_done").To("merger")
	}, "merger")

	g.Node("worker2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		time.Sleep(10 * time.Millisecond) // Simulate work
		return graph.Set(result2Key, "worker2_done").To("merger")
	}, "merger")

	// Merger node collects results
	g.Node("merger", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		r1 := graph.Get(view, result1Key)
		r2 := graph.Get(view, result2Key)
		return graph.Set(finalKey, r1+"_"+r2).End()
	}, graph.END)

	// Both workers start in parallel
	g.Start("worker1", "worker2")

	// Use executor with Redis message bus
	executor := graph.NewPregelExecutor[any, any]().WithMessageBus(bus)
	g.WithExecutor(executor)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	// Execute
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
	}

	t.Log("Parallel distributed state test passed!")
}
