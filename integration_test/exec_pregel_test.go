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

	g.AddNode(&graph.BaseNode{
		NodeName:        "start",
		DeclaredTargets: []string{"task1", "task2"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			counter.Add(1)
			updates, _ := graph.NewCommand().Set(state.NewKey("started", false), true).Build()
			return []string{"task1", "task2"}, updates, nil
		},
	})

	// Two nodes that can run in parallel
	g.AddNode(&graph.BaseNode{
		NodeName:        "task1",
		DeclaredTargets: []string{"end"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			counter.Add(1)
			updates, _ := graph.NewCommand().Set(state.NewKey("task1", ""), "done").Build()
			return []string{"end"}, updates, nil
		},
	})

	g.AddNode(&graph.BaseNode{
		NodeName:        "task2",
		DeclaredTargets: []string{"end"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			counter.Add(1)
			updates := state.Updates{"task2": "done"}
			return []string{"end"}, updates, nil
		},
	})

	g.AddNode(&graph.BaseNode{
		NodeName:        "end",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			counter.Add(1)
			updates := state.Updates{"completed": true}
			return []string{graph.EndNode}, updates, nil
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
