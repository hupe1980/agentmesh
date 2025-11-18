package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// registerMessagesKey is a helper to register the required __messages__ key
func registerMessagesKey(t *testing.T, mgr *state.Manager) {
	t.Helper()
	messagesKey := state.NewListKey[message.Message]("__messages__", 0)
	if err := state.RegisterListKey(mgr, messagesKey); err != nil {
		t.Fatalf("Failed to register messages key: %v", err)
	}
}

func TestBuilder_BasicUsage(t *testing.T) {
	// Define key first
	processedKey := state.NewKey("processed", false)

	builder, err := exec.NewBuilder(exec.NewPregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register key
	if err := state.RegisterKey(builder.Manager(), processedKey); err != nil {
		t.Fatalf("Failed to register key: %v", err)
	}

	// Add nodes using fluent API
	builder.
		Node("process", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"processed": true}, nil
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
	view, err := builder.Manager().CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}
	if !state.GetFromView(view, processedKey) {
		t.Error("Expected processed to be true")
	}
}

func TestBuilder_WithOptions(t *testing.T) {
	// Create builder
	stepKey := state.NewKey("step", 0)

	builder, err := exec.NewBuilder(exec.NewPregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register key
	if err := state.RegisterKey(builder.Manager(), stepKey); err != nil {
		t.Fatalf("Failed to register key: %v", err)
	}

	builder.
		Node("node1", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"step": 1}, nil
		}).
		Node("node2", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"step": 2}, nil
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

	view2, err := builder.Manager().CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}
	step := state.GetFromView(view2, stepKey)
	if step != 2 {
		t.Errorf("Expected step to be 2, got %v", step)
	}
}

func TestBuilder_ConditionalEdges(t *testing.T) {
	// Define keys first
	routeKey := state.NewKey("route", "")
	resultKey := state.NewKey("result", "")

	// Create builder
	builder, err := exec.NewBuilder(exec.NewPregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register keys
	if err := state.RegisterKey(builder.Manager(), routeKey); err != nil {
		t.Fatalf("Failed to register route key: %v", err)
	}
	if err := state.RegisterKey(builder.Manager(), resultKey); err != nil {
		t.Fatalf("Failed to register result key: %v", err)
	}

	builder.
		Node("router", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"route": "left"}, nil
		}).
		Node("left", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"result": "left"}, nil
		}).
		Node("right", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"result": "right"}, nil
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

	view3, err := builder.Manager().CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}
	result := state.GetFromView(view3, resultKey)
	if result != "left" {
		t.Errorf("Expected result to be 'left', got %v", result)
	}
}

func TestBuilder_ManualCompile(t *testing.T) {
	// Define key first
	doneKey := state.NewKey("done", false)

	// Test using exec.NewBuilder (recommended API)
	builder, err := exec.NewBuilder(exec.NewPregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register key
	if err := state.RegisterKey(builder.Manager(), doneKey); err != nil {
		t.Fatalf("Failed to register key: %v", err)
	}

	builder.
		Node("process", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"done": true}, nil
		}).
		AddEdge(graph.StartNode, "process").
		AddEdge("process", graph.EndNode)

	// Compile using the builder
	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()
	messages := []message.Message{message.NewHumanMessageFromText("test")}

	for range compiled.Run(ctx, messages) {
	}

	// Access state through Manager and ReadView
	view4, err := compiled.Manager().CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}
	done := state.GetFromView(view4, doneKey)
	if !done {
		t.Error("Expected done to be true")
	}
}
