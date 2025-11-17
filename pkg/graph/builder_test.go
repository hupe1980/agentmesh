package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func newTestState() *state.State {
	st := state.NewState()
	state.Register(st, state.MessagesKey.Key)
	return st
}

func TestBuilder_BasicUsage(t *testing.T) {
	// Define key first
	processedKey := state.NewKey("processed", false)

	// Create builder with custom state
	st := newTestState()
	state.Register(st, processedKey)

	builder, err := exec.NewBuilder(graph.WithState(st))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Add nodes using fluent API
	builder.
		Node("process", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
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

	// Verify the result by accessing state
	snap := builder.State().Snapshot()
	view := state.NewReadView(snap)
	if !state.GetFromView(view, processedKey) {
		t.Error("Expected processed to be true")
	}
}

func TestBuilder_WithOptions(t *testing.T) {
	// Create builder with custom state
	st := newTestState()
	stepKey := state.NewKey("step", 0)
	state.Register(st, stepKey)

	builder, err := exec.NewBuilder(graph.WithState(st))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.
		Node("node1", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"step": 1}}, nil
		}).
		Node("node2", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
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

	snap2 := builder.State().Snapshot()
	view2 := state.NewReadView(snap2)
	step := state.GetFromView(view2, stepKey)
	if step != 2 {
		t.Errorf("Expected step to be 2, got %v", step)
	}
}

func TestBuilder_ConditionalEdges(t *testing.T) {
	// Define keys first
	routeKey := state.NewKey("route", "")
	resultKey := state.NewKey("result", "")

	// Create builder with custom state
	st := newTestState()
	state.Register(st, routeKey)
	state.Register(st, resultKey)

	builder, err := exec.NewBuilder(graph.WithState(st))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.
		Node("router", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"route": "left"},
			}, nil
		}).
		Node("left", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"result": "left"}}, nil
		}).
		Node("right", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{Updates: map[string]any{"result": "right"}}, nil
		}).
		AddEdge(graph.StartNode, "router").
		AddConditionalEdges("router", func(ctx context.Context, s *state.ReadView) []string {
			route := state.GetFromView(s, routeKey)
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

	snap3 := builder.State().Snapshot()
	view3 := state.NewReadView(snap3)
	result := state.GetFromView(view3, resultKey)
	if result != "left" {
		t.Errorf("Expected result to be 'left', got %v", result)
	}
}

func TestBuilder_ManualCompile(t *testing.T) {
	// Define key first
	doneKey := state.NewKey("done", false)

	// Create state and register key
	st := newTestState()
	state.Register(st, doneKey)

	// Test using graph.NewBuilder without auto-compile
	builder, err := graph.NewBuilder(graph.WithState(st))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.
		Node("process", func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
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

	// Access state through snapshot and ReadView
	snap4 := g.State().Snapshot()
	view4 := state.NewReadView(snap4)
	done := state.GetFromView(view4, doneKey)
	if !done {
		t.Error("Expected done to be true")
	}
}
