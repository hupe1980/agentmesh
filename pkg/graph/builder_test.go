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
		AddStaticNode("process", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
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
		AddStaticNode("node1", []string{"node2"}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"step": 1}, nil
		}).
		AddStaticNode("node2", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
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
		AddNodeFunc("router", []string{"left", "right"}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			updates := map[string]any{"route": "left"}
			route := "left"
			return []string{route}, updates, nil
		}).
		AddStaticNode("left", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"result": "left"}, nil
		}).
		AddStaticNode("right", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
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
		AddStaticNode("process", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
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

// TestInterruptConfiguration verifies interrupt configuration
func TestInterruptConfiguration(t *testing.T) {
	manager := state.NewManager()
	g, err := graph.NewGraph(manager)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	node := &graph.BaseNode{
		NodeName:        "test",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, state.Updates{}, nil
		},
	}

	err = g.AddNode(node)
	if err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	g.AddInterruptBefore("test")

	if len(g.InterruptBefore) == 0 {
		t.Error("Expected InterruptBefore to contain 'test'")
	}
	found := false
	for _, name := range g.InterruptBefore {
		if name == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected InterruptBefore to contain 'test'")
	}
}

// TestResumeValueContext verifies resume values context handling
func TestResumeValueContext(t *testing.T) {
	ctx := context.Background()

	// Context without resume value should return nil
	resumeVal := graph.ResumeValueFromContext(ctx)
	if resumeVal != nil {
		t.Error("Expected nil resume value from empty context")
	}
}

// TestBuilder_MultipleEntryPoints verifies parallel execution from multiple entry points
func TestBuilder_MultipleEntryPoints(t *testing.T) {
	// Define keys for tracking execution
	taskAKey := state.NewKey("task_a", "")
	taskBKey := state.NewKey("task_b", "")
	mergeKey := state.NewKey("merged", "")

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Register messages key (required by executor)
	registerMessagesKey(t, builder.Manager())

	// Register keys
	if err := state.RegisterKey(builder.Manager(), taskAKey); err != nil {
		t.Fatalf("Failed to register task_a key: %v", err)
	}
	if err := state.RegisterKey(builder.Manager(), taskBKey); err != nil {
		t.Fatalf("Failed to register task_b key: %v", err)
	}
	if err := state.RegisterKey(builder.Manager(), mergeKey); err != nil {
		t.Fatalf("Failed to register merged key: %v", err)
	}

	// Add parallel task nodes using variadic SetEntryPoint
	builder.
		SetEntryPoint("task_a", "task_b").
		AddStaticNode("task_a", []string{"merge"}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"task_a": "result_a"}, nil
		}).
		AddStaticNode("task_b", []string{"merge"}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			return map[string]any{"task_b": "result_b"}, nil
		}).
		AddStaticNode("merge", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) (state.Updates, error) {
			resultA := state.GetFromView(s, taskAKey)
			resultB := state.GetFromView(s, taskBKey)
			return map[string]any{"merged": resultA + "_" + resultB}, nil
		})

	// Verify EntryPoints are set correctly
	g := builder.Graph()
	if len(g.EntryPoints) != 2 {
		t.Errorf("Expected 2 entry points, got %d", len(g.EntryPoints))
	}
	if g.EntryPoints[0] != "task_a" || g.EntryPoints[1] != "task_b" {
		t.Errorf("Expected entry points [task_a, task_b], got %v", g.EntryPoints)
	}

	// Compile and run
	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Execution error: %v", err)
		}
	}

	// Verify both tasks executed and merge combined results
	view, err := builder.Manager().CreateReadView(ctx)
	if err != nil {
		t.Fatalf("Failed to create read view: %v", err)
	}

	resultA := state.GetFromView(view, taskAKey)
	resultB := state.GetFromView(view, taskBKey)
	merged := state.GetFromView(view, mergeKey)

	if resultA != "result_a" {
		t.Errorf("Expected task_a result 'result_a', got %v", resultA)
	}
	if resultB != "result_b" {
		t.Errorf("Expected task_b result 'result_b', got %v", resultB)
	}
	if merged != "result_a_result_b" {
		t.Errorf("Expected merged result 'result_a_result_b', got %v", merged)
	}
}

// TestGraph_SetEntryPointMultipleCalls verifies calling SetEntryPoint multiple times appends
func TestGraph_SetEntryPointMultipleCalls(t *testing.T) {
	mgr := state.NewManager()
	g, err := graph.NewGraph(mgr)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	// Add nodes
	g.AddNode(&graph.BaseNode{
		NodeName:        "task_a",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})
	g.AddNode(&graph.BaseNode{
		NodeName:        "task_b",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})

	// Call SetEntryPoint multiple times
	if err := g.SetEntryPoint("task_a"); err != nil {
		t.Fatalf("Failed to set first entry point: %v", err)
	}
	if err := g.SetEntryPoint("task_b"); err != nil {
		t.Fatalf("Failed to set second entry point: %v", err)
	}

	// Verify both are stored
	if len(g.EntryPoints) != 2 {
		t.Fatalf("Expected 2 entry points, got %d", len(g.EntryPoints))
	}
	if g.EntryPoints[0] != "task_a" {
		t.Errorf("Expected first entry point 'task_a', got %v", g.EntryPoints[0])
	}
	if g.EntryPoints[1] != "task_b" {
		t.Errorf("Expected second entry point 'task_b', got %v", g.EntryPoints[1])
	}

	// Verify duplicate detection
	err = g.SetEntryPoint("task_a")
	if err == nil {
		t.Error("Expected error when adding duplicate entry point")
	}
}
