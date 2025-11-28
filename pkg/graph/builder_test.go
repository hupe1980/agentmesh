package graph_test

import (
	"context"
	"slices"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// registerMessagesKey is a helper to register the required __messages__ key
func registerMessagesKey(t *testing.T, builder *state.ManagerBuilder) {
	t.Helper()
	messagesKey := state.NewListKey[message.Message]("__messages__", 0)
	if err := state.RegisterListKey(builder, messagesKey); err != nil {
		t.Fatalf("Failed to register messages key: %v", err)
	}
}

// createManagerWithKeys creates a manager with registered keys for testing
func createManagerWithKeys(t *testing.T, keys ...any) *state.Manager {
	t.Helper()
	builder := state.NewManagerBuilder()
	registerMessagesKey(t, builder)
	for _, key := range keys {
		switch k := key.(type) {
		case state.Key[bool]:
			if err := state.RegisterKey(builder, k); err != nil {
				t.Fatalf("Failed to register key: %v", err)
			}
		case state.Key[string]:
			if err := state.RegisterKey(builder, k); err != nil {
				t.Fatalf("Failed to register key: %v", err)
			}
		case state.Key[int]:
			if err := state.RegisterKey(builder, k); err != nil {
				t.Fatalf("Failed to register key: %v", err)
			}
		}
	}
	return builder.Build()
}

func TestBuilder_BasicUsage(t *testing.T) {
	// Define key first
	processedKey := state.NewKey("processed", false)

	// Create and configure state manager
	mgr := createManagerWithKeys(t, processedKey)

	// Create builder with pre-configured manager
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Add nodes using fluent API
	builder.SetEntryPoint("process").
		AddNodeFunc("process", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, map[string]any{"processed": true}, nil
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

	// Create and configure state manager
	mgr := createManagerWithKeys(t, stepKey)

	// Create builder with pre-configured manager
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.SetEntryPoint("node1").
		AddNodeFunc("node1", []string{"node2"}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{"node2"}, map[string]any{"step": 1}, nil
		}).
		AddNodeFunc("node2", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, map[string]any{"step": 2}, nil
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

	// Create and configure state manager
	mgr := createManagerWithKeys(t, routeKey, resultKey)

	// Create builder with pre-configured manager
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.SetEntryPoint("router").
		AddNodeFunc("router", []string{"left", "right"}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			updates := map[string]any{"route": "left"}
			route := "left"
			return []string{route}, updates, nil
		}).
		AddNodeFunc("left", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, map[string]any{"result": "left"}, nil
		}).
		AddNodeFunc("right", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, map[string]any{"result": "right"}, nil
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

	// Create and configure state manager
	mgr := createManagerWithKeys(t, doneKey)

	// Test using graph.NewBuilder (recommended API)
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.SetEntryPoint("process").
		AddNodeFunc("process", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, map[string]any{"done": true}, nil
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
	manager := state.NewManagerBuilder().Build()
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
	found := slices.Contains(g.InterruptBefore, "test")
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

	// Create and configure state manager
	mgr := createManagerWithKeys(t, taskAKey, taskBKey, mergeKey)

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Add parallel task nodes using variadic SetEntryPoint
	builder.
		SetEntryPoint("task_a", "task_b").
		AddNodeFunc("task_a", []string{"merge"}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{"merge"}, map[string]any{"task_a": "result_a"}, nil
		}).
		AddNodeFunc("task_b", []string{"merge"}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return []string{"merge"}, map[string]any{"task_b": "result_b"}, nil
		}).
		AddNodeFunc("merge", []string{graph.EndNode}, func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			resultA := state.GetFromView(s, taskAKey)
			resultB := state.GetFromView(s, taskBKey)
			return []string{graph.EndNode}, map[string]any{"merged": resultA + "_" + resultB}, nil
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
	mgr := state.NewManagerBuilder().Build()
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

func TestBuilder_WithManager(t *testing.T) {
	// Create custom state manager
	customManager := state.NewManagerBuilder().Build()

	// Create builder with custom manager
	builder, err := graph.NewBuilder(
		graph.NewMessagePregelExecutor(),
		graph.WithManager[[]message.Message, message.Message](customManager),
	)
	if err != nil {
		t.Fatalf("Failed to create builder with manager: %v", err)
	}

	// Verify the manager is the one we provided
	if builder.Manager() != customManager {
		t.Error("Expected builder to use custom manager")
	}
}

func TestBuilder_WithInterruptBefore(t *testing.T) {
	builder, err := graph.NewBuilder(
		graph.NewMessagePregelExecutor(),
		graph.WithInterruptBefore[[]message.Message, message.Message]("node1", "node2"),
	)
	if err != nil {
		t.Fatalf("Failed to create builder with interrupt before: %v", err)
	}

	g := builder.Graph()
	if len(g.InterruptBefore) != 2 {
		t.Errorf("Expected 2 interrupt before nodes, got %d", len(g.InterruptBefore))
	}
	if !slices.Contains(g.InterruptBefore, "node1") {
		t.Error("Expected node1 in InterruptBefore")
	}
	if !slices.Contains(g.InterruptBefore, "node2") {
		t.Error("Expected node2 in InterruptBefore")
	}
}

func TestBuilder_WithInterruptAfter(t *testing.T) {
	builder, err := graph.NewBuilder(
		graph.NewMessagePregelExecutor(),
		graph.WithInterruptAfter[[]message.Message, message.Message]("node1", "node2"),
	)
	if err != nil {
		t.Fatalf("Failed to create builder with interrupt after: %v", err)
	}

	g := builder.Graph()
	if len(g.InterruptAfter) != 2 {
		t.Errorf("Expected 2 interrupt after nodes, got %d", len(g.InterruptAfter))
	}
	if !slices.Contains(g.InterruptAfter, "node1") {
		t.Error("Expected node1 in InterruptAfter")
	}
	if !slices.Contains(g.InterruptAfter, "node2") {
		t.Error("Expected node2 in InterruptAfter")
	}
}

func TestBuilder_AddNode_CustomNode(t *testing.T) {
	// Create and configure state manager
	mgr := createManagerWithKeys(t)

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Create custom node
	customNode := &graph.BaseNode{
		NodeName:        "custom",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, state.Updates{"custom": true}, nil
		},
	}

	// Add custom node
	builder.SetEntryPoint("custom").AddNode(customNode)

	// Compile and verify
	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	// Verify node was added
	nodes := compiled.GetNodes()
	if len(nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(nodes))
	}
	if nodes[0] != "custom" {
		t.Errorf("Expected node name 'custom', got %s", nodes[0])
	}
}

func TestBuilder_ErrorAccumulation(t *testing.T) {
	// Create and configure state manager
	mgr := createManagerWithKeys(t)

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Try to set a non-existent node as entry point
	builder.SetEntryPoint("nonexistent")

	// Compile should fail with accumulated error
	_, err = builder.Compile()
	if err == nil {
		t.Error("Expected error from compilation due to invalid entry point")
	}
}

func TestBuilder_Graph(t *testing.T) {
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	g := builder.Graph()
	if g == nil {
		t.Error("Expected non-nil graph")
	}
}

func TestBuilder_Manager(t *testing.T) {
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	mgr := builder.Manager()
	if mgr == nil {
		t.Error("Expected non-nil manager")
	}
}

func TestBuilder_ChainedMethods(t *testing.T) {
	// Create and configure state manager
	mgr := createManagerWithKeys(t)

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Chain multiple methods
	result := builder.
		SetEntryPoint("start").
		AddNodeFunc("start", []string{"middle"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"middle"}, state.Updates{"step": 1}, nil
		}).
		AddNodeFunc("middle", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, state.Updates{"step": 2}, nil
		})

	// Verify chaining returns the same builder
	if result != builder {
		t.Error("Expected chained methods to return the same builder")
	}

	// Verify compilation works
	_, err = builder.Compile()
	if err != nil {
		t.Errorf("Compilation failed: %v", err)
	}
}

func TestBuilder_CompileWithoutNodes(t *testing.T) {
	// Create and configure state manager
	mgr := createManagerWithKeys(t)

	// Create empty builder
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	// Try to compile without setting entry point or adding nodes
	_, err = builder.Compile()
	if err == nil {
		t.Error("Expected error when compiling without entry points")
	}
}
