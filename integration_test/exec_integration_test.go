package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestNewArchitecture(t *testing.T) {
	// Step 1: Build a simple graph
	stateManager, err := state.NewStateManager(100)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	g, err := graph.NewGraph(stateManager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	g.AddNode(&graph.Node{
		Name: "start",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"step": "started"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "process",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"step": "processed"},
				Messages: []message.Message{
					message.NewHumanMessageFromText("Processing complete"),
				},
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
	for result, err := range runnable.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
		resultCount++
		t.Logf("Result %d: Node=%s, Message=%v", resultCount, result.Node, result.Message)
	}

	// Verify state was updated
	val := stateManager.Get("step")
	if val != "processed" {
		t.Errorf("Expected step='processed', got %v", val)
	}

	// Verify messages were added
	messages, exists := stateManager.GetChannel("messages")
	if !exists || messages == nil {
		t.Error("Expected messages channel to exist")
	}

	t.Logf("✅ New architecture test passed! Generated %d results", resultCount)
}
