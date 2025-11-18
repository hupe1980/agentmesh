package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestNewArchitecture(t *testing.T) {
	// Define typed keys
	stepKey := state.NewKey("step", "")

	// Step 1: Build a simple graph
	stateManager := newTestManager()
	state.RegisterKey(stateManager, stepKey)

	g, err := graph.NewGraph(stateManager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	g.AddNode(&graph.Node{
		Name: "start",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"step": "started"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "process",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"step": "processed"},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "start")
	g.AddEdge("start", "process")
	g.AddEdge("process", graph.EndNode)

	// Step 2: Compile and execute the graph
	runnable, err := exec.CompileGraph(g, exec.WithExecutor(exec.NewSequential()))
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	// Step 3: Execute
	ctx := context.Background()

	resultCount := 0
	for result := range runnable.Run(ctx, nil) {
		resultCount++
		t.Logf("Result %d: Message=%v", resultCount, result)
	}

	// Verify state was updated
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}
	val := state.GetFromView(view, stepKey)
	if val != "processed" {
		t.Errorf("Expected step='processed', got %v", val)
	}

	t.Logf("✅ New architecture test passed! Generated %d results", resultCount)
}
