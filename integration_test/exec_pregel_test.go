package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
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
	stateManager := newTestState()
	state.Register(stateManager, startedKey)
	state.Register(stateManager, task1Key)
	state.Register(stateManager, task2Key)
	state.Register(stateManager, completedKey)

	g, err := graph.NewGraph(stateManager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	var counter atomic.Int32

	g.AddNode(&graph.Node{
		Name: "start",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			counter.Add(1)
			return &graph.NodeResult{
				Updates: map[string]any{"started": true},
			}, nil
		},
	})

	// Two nodes that can run in parallel
	g.AddNode(&graph.Node{
		Name: "task1",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			counter.Add(1)
			return &graph.NodeResult{
				Updates: map[string]any{"task1": "done"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "task2",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			counter.Add(1)
			return &graph.NodeResult{
				Updates: map[string]any{"task2": "done"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "end",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			counter.Add(1)
			return &graph.NodeResult{
				Updates: map[string]any{"completed": true},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "start")
	g.AddEdge("start", "task1")
	g.AddEdge("start", "task2")
	g.AddEdge("task1", "end")
	g.AddEdge("task2", "end")
	g.AddEdge("end", graph.EndNode)

	// Compile and execute with Pregel
	runnable, err := exec.CompileGraph(g, exec.WithExecutor(exec.NewPregelExecutor(exec.WithMaxWorkers(4))))
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()

	resultCount := 0
	for result, err := range runnable.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
		resultCount++
		t.Logf("Result %d: Node=%s", resultCount, result.Node)
	}

	// Verify all nodes executed
	if counter.Load() != 4 {
		t.Errorf("Expected 4 nodes to execute, got %d", counter.Load())
	}

	// Verify final state
	snap := stateManager.Snapshot()
	view := state.NewReadView(snap)
	if val := state.GetFromView(view, completedKey); val != true {
		t.Errorf("Expected completed=true, got %v", val)
	}

	t.Logf("✅ Pregel executor test passed! Executed %d nodes", counter.Load())
}
