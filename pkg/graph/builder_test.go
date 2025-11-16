package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestBuilder_BasicUsage(t *testing.T) {
	// Create builder with exec.NewBuilder for automatic compilation
	builder, err := exec.NewBuilder()
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Add nodes using fluent API
	builder.
		Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"processed": true},
			}, nil
		}).
		AddEdge(graph.StartNode, "process").
		AddEdge("process", graph.EndNode)

	// Compile the graph
	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	// Run the graph
	ctx := context.Background()
	messages := []message.Message{message.NewHumanMessageFromText("test")}

	for range compiled.Run(ctx, messages) {
	}

	// Verify the result by accessing state manager
	stateManager := builder.StateManager()
	if !stateManager.Get("processed").(bool) {
		t.Error("Expected processed to be true")
	}
}

func TestBuilder_WithOptions(t *testing.T) {
	// Create builder with custom state manager
	stateManager, err := state.NewStateManager(10)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	builder, err := exec.NewBuilder(graph.WithStateManager(stateManager))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.
		Node("node1", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"step": 1}}, nil
		}).
		Node("node2", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"step": 2}}, nil
		}).
		AddEdge(graph.StartNode, "node1").
		AddEdge("node1", "node2").
		AddEdge("node2", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()
	messages := []message.Message{message.NewHumanMessageFromText("test")}

	for range compiled.Run(ctx, messages) {
	}

	stateManager2 := builder.StateManager()
	if stateManager2.Get("step").(int) != 2 {
		t.Errorf("Expected step to be 2, got %v", stateManager2.Get("step"))
	}
}

func TestBuilder_ConditionalEdges(t *testing.T) {
	builder, err := exec.NewBuilder()
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.
		Node("router", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"route": "left"},
			}, nil
		}).
		Node("left", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"result": "left"}}, nil
		}).
		Node("right", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"result": "right"}}, nil
		}).
		AddEdge(graph.StartNode, "router").
		AddConditionalEdges("router", func(ctx context.Context, s state.Reader) []string {
			route := s.Get("route").(string)
			if route == "left" {
				return []string{"left"}
			}
			return []string{"right"}
		}, []string{"left", "right"}).
		AddEdge("left", graph.EndNode).
		AddEdge("right", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()
	messages := []message.Message{message.NewHumanMessageFromText("test")}

	for range compiled.Run(ctx, messages) {
	}

	stateManager3 := builder.StateManager()
	if stateManager3.Get("result").(string) != "left" {
		t.Errorf("Expected result to be 'left', got %v", stateManager3.Get("result"))
	}
}

func TestBuilder_ManualCompile(t *testing.T) {
	// Test using graph.NewBuilder without auto-compile
	builder, err := graph.NewBuilder()
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.
		Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"done": true}}, nil
		}).
		AddEdge(graph.StartNode, "process").
		AddEdge("process", graph.EndNode)

	// Compile manually using exec.CompileGraph
	g := builder.Build()
	compiled, err := exec.CompileGraph(g)
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()
	messages := []message.Message{message.NewHumanMessageFromText("test")}

	for range compiled.Run(ctx, messages) {
	}

	// Access state through the state manager
	gState := g.StateManager()
	if !gState.Get("done").(bool) {
		t.Error("Expected done to be true")
	}
}
