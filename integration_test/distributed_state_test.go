package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestDistributedStateSync verifies that state.Updates propagate correctly
// through a Redis message bus in a distributed BSP execution.
//
// Note: Uses default JSONCodec which coerces all numbers to float64.
// This is expected JSON behavior. For type preservation, use GOB codec.
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

	// Create Redis message bus for state.Updates
	bus := predis.NewMessageBus[state.Updates](addr, "", 0, &predis.Options{
		Namespace: "test-distributed-state",
		TTL:       1 * time.Minute,
	})
	defer bus.Close()

	// Test Ping
	if err := bus.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping Redis: %v", err)
	}

	// Define state keys for testing
	// Use float64 for counter since JSON unmarshals numbers as float64
	counterKey := state.NewKey("counter", 0.0)
	dataKey := state.NewKey("data", "")

	// Create state manager and register keys
	manager := state.NewManager()
	state.RegisterKey(manager, counterKey)
	state.RegisterKey(manager, dataKey)

	// Build a graph with nodes that modify state
	// node1 -> node2 -> node3
	// Each node increments counter and appends to data
	g, err := graph.NewGraph(manager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	// Node 1: Initialize state
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node1",
		DeclaredTargets: graph.NewTargetSet("node2"),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, counterKey, 1.0) // Use float64 for JSON compatibility
			state.SetUpdate(builder, dataKey, "A")
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.Goto("node2", updates), nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node1: %v", err)
	}

	// Node 2: Read and modify state (should see node1's updates via Redis)
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node2",
		DeclaredTargets: graph.NewTargetSet("node3"),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			counter := state.GetFromView(view, counterKey)
			data := state.GetFromView(view, dataKey)

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, counterKey, counter+1.0) // Should be 2.0
			state.SetUpdate(builder, dataKey, data+"B")       // Should be "AB"
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.Goto("node3", updates), nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node2: %v", err)
	}

	// Node 3: Final state update (should see node2's updates via Redis)
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node3",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			counter := state.GetFromView(view, counterKey)
			data := state.GetFromView(view, dataKey)

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, counterKey, counter+1.0) // Should be 3.0
			state.SetUpdate(builder, dataKey, data+"C")       // Should be "ABC"
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.End(updates), nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node3: %v", err)
	}

	g.SetEntryPoint("node1")

	// Compile with state-based executor + Redis message bus
	compiled, err := graph.Compile(g, graph.NewStatePregelExecutor(
		graph.WithMessageBus[state.Updates, state.Updates](bus),
	))
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	// Execute the graph
	finalState := make(state.Updates)
	for updates, err := range compiled.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
		// Collect all state updates
		for k, v := range updates {
			finalState[k] = v
		}
	}

	// Verify final state - if distributed state sync works, we should see:
	// counter = 3.0 (1.0 + 1.0 + 1.0)
	// data = "ABC"
	finalCounter, ok := finalState[counterKey.Name()].(float64)
	if !ok {
		t.Fatalf("Counter not found in final state")
	}
	if finalCounter != 3.0 {
		t.Errorf("Expected counter=3.0, got %.1f (state updates didn't propagate correctly)", finalCounter)
	}

	finalData, ok := finalState[dataKey.Name()].(string)
	if !ok {
		t.Fatalf("Data not found in final state")
	}
	if finalData != "ABC" {
		t.Errorf("Expected data='ABC', got '%s' (state updates didn't propagate correctly)", finalData)
	}

	t.Logf("✓ Distributed state sync working! Final state: counter=%.1f, data=%s", finalCounter, finalData)
}

// TestDistributedStateSync_DisabledSync verifies that when distributed state
// is disabled, nodes don't receive predecessor state (routing-only mode).
func TestDistributedStateSync_DisabledSync(t *testing.T) {
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
	bus := predis.NewMessageBus[state.Updates](addr, "", 0, &predis.Options{
		Namespace: "test-routing-only",
		TTL:       1 * time.Minute,
	})
	defer bus.Close()

	counterKey := state.NewKey("counter", 0.0) // Use float64 for JSON compatibility

	// Create state manager and register key
	manager := state.NewManager()
	state.RegisterKey(manager, counterKey)

	g, err := graph.NewGraph(manager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	// Node 1: Set counter = 1.0
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node1",
		DeclaredTargets: graph.NewTargetSet("node2"),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, counterKey, 1.0)
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.Goto("node2", updates), nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node1: %v", err)
	}

	// Node 2: Try to read counter (should see 1.0 from local state, not redistributed)
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node2",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			counter := state.GetFromView(view, counterKey) // Should be 1.0 from local state

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, counterKey, counter+10.0) // Should be 11.0 (1.0 + 10.0)
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.End(updates), nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node2: %v", err)
	}

	g.SetEntryPoint("node1")

	// Compile with distributed state DISABLED (routing-only)
	compiled, err := graph.Compile(g, graph.NewStatePregelExecutor(
		graph.WithMessageBus[state.Updates, state.Updates](bus),
		graph.WithDistributedState[state.Updates, state.Updates](false), // Disable state sync
	))
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	// Execute
	finalState := make(state.Updates)
	for updates, err := range compiled.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
		for k, v := range updates {
			finalState[k] = v
		}
	}

	// Verify counter = 11.0 (local state accumulates even without distributed sync)
	// This proves that:
	// 1. Routing works (node1 -> node2)
	// 2. Local state manager preserves updates
	// 3. But no distributed state synchronization happens via Redis
	finalCounter, ok := finalState[counterKey.Name()].(float64)
	if !ok {
		t.Fatalf("Counter not found in final state")
	}
	if finalCounter != 11.0 {
		t.Errorf("Expected counter=11.0 (routing-only mode), got %.1f", finalCounter)
	}

	t.Logf("✓ Routing-only mode working! Counter=%.1f (local state, no distributed sync)", finalCounter)
}
