package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestPregelExecutor(t *testing.T) {
	// Define typed keys
	startedKey := state.NewKey("started", false)
	task1Key := state.NewKey("task1", "")
	task2Key := state.NewKey("task2", "")
	completedKey := state.NewKey("completed", false)

	// Build a graph with parallel nodes
	stateManager := newTestManager()
	state.RegisterKey(stateManager, startedKey)
	state.RegisterKey(stateManager, task1Key)
	state.RegisterKey(stateManager, task2Key)
	state.RegisterKey(stateManager, completedKey)

	g, err := graph.NewGraph(stateManager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	var counter atomic.Int32

	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "start",
		DeclaredTargets: graph.NewTargetSet("task1", "task2"),
		Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
			counter.Add(1)
			updates := map[string]any{"started": true}
			return graph.GotoAll([]string{"task1", "task2"}, updates), nil
		},
	})

	// Two nodes that can run in parallel
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "task1",
		DeclaredTargets: graph.NewTargetSet("end"),
		Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
			counter.Add(1)
			updates := map[string]any{"task1": "done"}
			return graph.Goto("end", updates), nil
		},
	})

	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "task2",
		DeclaredTargets: graph.NewTargetSet("end"),
		Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
			counter.Add(1)
			updates := map[string]any{"task2": "done"}
			return graph.Goto("end", updates), nil
		},
	})

	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "end",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
			counter.Add(1)
			updates := map[string]any{"completed": true}
			return graph.End(updates), nil
		},
	})

	g.SetEntryPoint("start")

	// Compile and execute with Pregel
	runnable, err := graph.Compile(g, graph.NewStatePregelExecutor(graph.WithMaxWorkers[state.Updates, state.Updates](4)))
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()

	resultCount := 0
	for result := range runnable.Run(ctx, nil) {
		resultCount++
		t.Logf("Result %d: %v", resultCount, result)
	}

	// Verify all nodes executed
	if counter.Load() != 4 {
		t.Errorf("Expected 4 nodes to execute, got %d", counter.Load())
	}

	// Verify final state
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}
	if val := state.GetFromView(view, completedKey); val != true {
		t.Errorf("Expected completed=true, got %v", val)
	}

	t.Logf("✅ Pregel executor test passed! Executed %d nodes", counter.Load())
}
