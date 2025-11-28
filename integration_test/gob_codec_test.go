package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestGOBCodec_TypePreservation verifies that GOBCodec preserves exact Go types
// (int stays int, not float64 like JSON).
func TestGOBCodec_TypePreservation(t *testing.T) {
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

	// Create Redis message bus with GOB codec
	bus := predis.NewMessageBus[state.Updates](addr, "", 0, &predis.Options{
		Namespace: "test-gob-codec",
		TTL:       1 * time.Minute,
		Codec:     pregel.NewGOBCodec(), // Use GOB for type preservation
	})
	defer bus.Close()

	// Use int (not float64) to test type preservation
	counterKey := state.NewKey("counter", 0) // int default
	dataKey := state.NewKey("data", "")

	// Create state manager and register keys
	builder := state.NewManagerBuilder()
	state.RegisterKey(builder, counterKey)
	state.RegisterKey(builder, dataKey)
	manager := builder.Build()

	// Create graph with 3 nodes
	g, err := graph.NewGraph(manager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	// Node 1: Initialize state with int
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "node1",
		DeclaredTargets: []string{"node2"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			updates := state.Updates{}
			updates[counterKey.Name()] = 1 // int, not float64
			updates[dataKey.Name()] = "A"
			return []string{"node2"}, updates, nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node1: %v", err)
	}

	// Node 2: Increment counter (should stay int)
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "node2",
		DeclaredTargets: []string{"node3"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			counter := state.GetFromView(view, counterKey)
			data := state.GetFromView(view, dataKey)

			updates := state.Updates{}
			updates[counterKey.Name()] = counter + 1 // Should be int 2
			updates[dataKey.Name()] = data + "B"
			return []string{"node3"}, updates, nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node2: %v", err)
	}

	// Node 3: Final increment
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "node3",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			counter := state.GetFromView(view, counterKey)
			data := state.GetFromView(view, dataKey)

			updates := state.Updates{}
			updates[counterKey.Name()] = counter + 1 // Should be int 3
			updates[dataKey.Name()] = data + "C"
			return []string{graph.EndNode}, updates, nil
		},
	})
	if err != nil {
		t.Fatalf("Failed to add node3: %v", err)
	}

	g.SetEntryPoint("node1")

	// Compile with state-based executor + Redis message bus with GOB codec
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

	// Verify counter is int (not float64) - GOB preserves exact types
	finalCounter, ok := finalState[counterKey.Name()].(int)
	if !ok {
		// If this fails, it means GOB didn't preserve the int type
		t.Fatalf("GOB codec failed to preserve int type! Got: %T (value: %v)",
			finalState[counterKey.Name()], finalState[counterKey.Name()])
	}
	if finalCounter != 3 {
		t.Errorf("Expected counter=3, got %d", finalCounter)
	}

	finalData, ok := finalState[dataKey.Name()].(string)
	if !ok {
		t.Fatalf("Data not found in final state")
	}
	if finalData != "ABC" {
		t.Errorf("Expected data='ABC', got '%s'", finalData)
	}

	t.Logf("✓ GOB codec preserves types! Final state: counter=%d (int), data=%s", finalCounter, finalData)
}
