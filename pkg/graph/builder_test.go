package graph_test

import (
	"context"
	"testing"

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

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
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
	builder.SetEntryPoint("process").
		AddStaticNode("process", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"processed": true}, nil
		})

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

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register key
	if err := state.RegisterKey(builder.Manager(), stepKey); err != nil {
		t.Fatalf("Failed to register key: %v", err)
	}

	builder.SetEntryPoint("node1").
		AddStaticNode("node1", graph.NewTargetSet("node2"), func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"step": 1}, nil
		}).
		AddStaticNode("node2", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"step": 2}, nil
		})

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
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
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

	builder.SetEntryPoint("router").
		AddCommandNode("router", graph.NewTargetSet("left", "right"), func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
			updates := map[string]any{"route": "left"}
			route := "left"
			return graph.Goto(route, updates), nil
		}).
		AddStaticNode("left", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"result": "left"}, nil
		}).
		AddStaticNode("right", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"result": "right"}, nil
		})

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

	// Test using graph.NewBuilder (recommended API)
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register key
	if err := state.RegisterKey(builder.Manager(), doneKey); err != nil {
		t.Fatalf("Failed to register key: %v", err)
	}

	builder.SetEntryPoint("process").
		AddStaticNode("process", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"done": true}, nil
		})

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
