package graph_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestSubgraphNode_BasicFunctionality(t *testing.T) {
	// Test that SubgraphNode implements Node interface
	ctx := context.Background()

	// Build a simple subgraph
	subBuilder := state.NewManagerBuilder()
	dataKey := state.NewKey[string]("data", "")
	state.RegisterKey(subBuilder, dataKey)
	subManager := subBuilder.Build()

	subGraph, _ := graph.NewGraph(subManager)
	subGraph.AddNode(&graph.BaseNode{
		NodeName:        "process",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			data := state.GetFromView(view, dataKey)
			return []string{graph.EndNode}, state.Updates{
				dataKey.Name(): data + "_processed",
			}, nil
		},
	})
	subGraph.SetEntryPoint("process")

	// Create executor for subgraph
	outputKey := "result"
	executor := graph.NewPregelExecutor(
		func(input string) state.Updates {
			return state.Updates{dataKey.Name(): input}
		},
		outputKey,
		func(val any) string {
			if s, ok := val.(string); ok {
				return s
			}
			return ""
		},
	)

	compiled, err := graph.Compile(subGraph, executor)
	if err != nil {
		t.Fatalf("Failed to compile subgraph: %v", err)
	}

	// Create SubgraphNode
	subgraphNode := graph.NewSubgraphNode(
		"subgraph",
		compiled,
		func(ctx context.Context, view state.ReadView) (string, error) {
			// Simple input mapper
			return "test_input", nil
		},
		func(ctx context.Context, output string) (state.Updates, error) {
			// Simple output mapper
			return state.Updates{"output": output}, nil
		},
		[]string{graph.EndNode},
	)

	// Verify Node interface methods
	if subgraphNode.Name() != "subgraph" {
		t.Errorf("Expected name 'subgraph', got: %s", subgraphNode.Name())
	}

	targets := subgraphNode.Targets()
	if len(targets) != 1 || targets[0] != graph.EndNode {
		t.Errorf("Expected targets [%s], got: %v", graph.EndNode, targets)
	}

	// Execute the node
	emptyManager := state.NewManagerBuilder().Build()
	view, _ := emptyManager.CreateReadView(ctx)

	returnedTargets, updates, err := subgraphNode.Execute(ctx, view)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(returnedTargets) != 1 || returnedTargets[0] != graph.EndNode {
		t.Errorf("Expected returned targets [%s], got: %v", graph.EndNode, returnedTargets)
	}

	// The output should be set by the output mapper
	output := updates["output"]
	if output == nil {
		t.Error("Expected output to be set")
	}
}

func TestSubgraphNode_InputMapperError(t *testing.T) {
	ctx := context.Background()

	// Build minimal subgraph
	subManager := state.NewManagerBuilder().Build()
	subGraph, _ := graph.NewGraph(subManager)
	subGraph.AddNode(&graph.BaseNode{
		NodeName:        "dummy",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})
	subGraph.SetEntryPoint("dummy")

	executor := graph.NewPregelExecutor(
		func(input string) state.Updates { return nil },
		"output",
		func(val any) string { return "" },
	)
	compiled, _ := graph.Compile(subGraph, executor)

	// Input mapper that returns error
	subgraphNode := graph.NewSubgraphNode(
		"subgraph",
		compiled,
		func(ctx context.Context, view state.ReadView) (string, error) {
			return "", fmt.Errorf("input mapping failed")
		},
		func(ctx context.Context, output string) (state.Updates, error) {
			return state.Updates{}, nil
		},
		[]string{graph.EndNode},
	)

	emptyManager := state.NewManagerBuilder().Build()
	view, _ := emptyManager.CreateReadView(ctx)

	_, _, err := subgraphNode.Execute(ctx, view)
	if err == nil {
		t.Error("Expected error from input mapper, got nil")
	}
	if !contains(err.Error(), "input mapping failed") {
		t.Errorf("Expected error to contain 'input mapping failed', got: %v", err)
	}
}

func TestSubgraphNode_OutputMapperError(t *testing.T) {
	ctx := context.Background()

	// Build minimal subgraph
	subManager := state.NewManagerBuilder().Build()
	subGraph, _ := graph.NewGraph(subManager)
	subGraph.AddNode(&graph.BaseNode{
		NodeName:        "dummy",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})
	subGraph.SetEntryPoint("dummy")

	executor := graph.NewPregelExecutor(
		func(input string) state.Updates { return nil },
		"output",
		func(val any) string { return "result" },
	)
	compiled, _ := graph.Compile(subGraph, executor)

	// Output mapper that returns error
	subgraphNode := graph.NewSubgraphNode(
		"subgraph",
		compiled,
		func(ctx context.Context, view state.ReadView) (string, error) {
			return "input", nil
		},
		func(ctx context.Context, output string) (state.Updates, error) {
			return nil, fmt.Errorf("output mapping failed")
		},
		[]string{graph.EndNode},
	)

	emptyManager := state.NewManagerBuilder().Build()
	view, _ := emptyManager.CreateReadView(ctx)

	_, _, err := subgraphNode.Execute(ctx, view)
	if err == nil {
		t.Error("Expected error from output mapper, got nil")
	}
	if !contains(err.Error(), "output mapping failed") {
		t.Errorf("Expected error to contain 'output mapping failed', got: %v", err)
	}
}

func TestSubgraphNode_Metadata(t *testing.T) {
	// Build minimal subgraph for testing metadata
	subManager := state.NewManagerBuilder().Build()
	subGraph, _ := graph.NewGraph(subManager)
	subGraph.AddNode(&graph.BaseNode{
		NodeName:        "dummy",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})
	subGraph.SetEntryPoint("dummy")

	executor := graph.NewPregelExecutor(
		func(input string) state.Updates { return nil },
		"output",
		func(val any) string { return "" },
	)
	compiled, _ := graph.Compile(subGraph, executor)

	subgraphNode := graph.NewSubgraphNode(
		"validation",
		compiled,
		func(ctx context.Context, view state.ReadView) (string, error) { return "", nil },
		func(ctx context.Context, output string) (state.Updates, error) { return state.Updates{}, nil },
		[]string{graph.EndNode},
		graph.WithSubgraphVersion("1.2.3"),
		graph.WithSubgraphMetadata("author", "test-team"),
		graph.WithSubgraphMetadata("description", "Test subgraph"),
	)

	if subgraphNode.Version() != "1.2.3" {
		t.Errorf("Expected version 1.2.3, got: %s", subgraphNode.Version())
	}

	metadata := subgraphNode.Metadata()
	if metadata["author"] != "test-team" {
		t.Errorf("Expected author=test-team, got: %s", metadata["author"])
	}
	if metadata["description"] != "Test subgraph" {
		t.Errorf("Expected description='Test subgraph', got: %s", metadata["description"])
	}
}

func TestSubgraphNode_RetryPolicy(t *testing.T) {
	// Build minimal subgraph
	subManager := state.NewManagerBuilder().Build()
	subGraph, _ := graph.NewGraph(subManager)
	subGraph.AddNode(&graph.BaseNode{
		NodeName:        "dummy",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})
	subGraph.SetEntryPoint("dummy")

	executor := graph.NewPregelExecutor(
		func(input string) state.Updates { return nil },
		"output",
		func(val any) string { return "" },
	)
	compiled, _ := graph.Compile(subGraph, executor)

	retryPolicy := graph.NewRetryPolicy().WithMaxAttempts(5).Build()
	subgraphNode := graph.NewSubgraphNode(
		"validation",
		compiled,
		func(ctx context.Context, view state.ReadView) (string, error) { return "", nil },
		func(ctx context.Context, output string) (state.Updates, error) { return state.Updates{}, nil },
		[]string{graph.EndNode},
		graph.WithSubgraphRetry(retryPolicy),
	)

	if subgraphNode.RetryPolicy() == nil {
		t.Error("Expected retry policy to be set")
	}
	if subgraphNode.RetryPolicy().MaxAttempts != 5 {
		t.Errorf("Expected max attempts=5, got %d", subgraphNode.RetryPolicy().MaxAttempts)
	}
}

func TestSubgraphNode_SimpleMappers(t *testing.T) {
	ctx := context.Background()

	// Test SimpleInputMapper
	dataKey := state.NewKey[string]("data", "default")
	inputMapper := graph.SimpleInputMapper(dataKey)

	builder := state.NewManagerBuilder()
	state.RegisterKey(builder, dataKey)
	manager := builder.Build()
	manager.ApplyUpdates(ctx, state.Updates{dataKey.Name(): "test value"})

	view, _ := manager.CreateReadView(ctx)
	result, err := inputMapper(ctx, view)
	if err != nil {
		t.Fatalf("SimpleInputMapper failed: %v", err)
	}
	if result != "test value" {
		t.Errorf("Expected 'test value', got: %s", result)
	}

	// Test SimpleOutputMapper
	resultKey := state.NewKey[string]("result", "")
	outputMapper := graph.SimpleOutputMapper(resultKey)

	updates, err := outputMapper(ctx, "output value")
	if err != nil {
		t.Fatalf("SimpleOutputMapper failed: %v", err)
	}
	if updates[resultKey.Name()] != "output value" {
		t.Errorf("Expected 'output value', got: %v", updates[resultKey.Name()])
	}
}

func TestSubgraphNode_PassthroughMappers(t *testing.T) {
	ctx := context.Background()

	// Test PassthroughInputMapper
	inputMapper := graph.PassthroughInputMapper("test input")
	result, err := inputMapper(ctx, nil)
	if err != nil {
		t.Fatalf("PassthroughInputMapper failed: %v", err)
	}
	if result != "test input" {
		t.Errorf("Expected 'test input', got: %s", result)
	}

	// Test PassthroughOutputMapper
	outputMapper := graph.PassthroughOutputMapper()
	updates := state.Updates{"key": "value"}

	result2, err := outputMapper(ctx, updates)
	if err != nil {
		t.Fatalf("PassthroughOutputMapper failed: %v", err)
	}
	if result2["key"] != "value" {
		t.Errorf("Expected 'value', got: %v", result2["key"])
	}
}

func TestSubgraphNode_Integration(t *testing.T) {
	// Simplified integration test: verify SubgraphNode can be added to builder

	// Build a simple subgraph
	subBuilder := state.NewManagerBuilder()
	dataKey := state.NewKey[string]("data", "")
	state.RegisterKey(subBuilder, dataKey)
	subManager := subBuilder.Build()

	subGraph, _ := graph.NewGraph(subManager)
	subGraph.AddNode(&graph.BaseNode{
		NodeName:        "process",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, state.Updates{
				dataKey.Name(): "processed",
			}, nil
		},
	})
	subGraph.SetEntryPoint("process")

	executor := graph.NewPregelExecutor(
		func(input string) state.Updates {
			return state.Updates{dataKey.Name(): input}
		},
		dataKey.Name(),
		func(val any) string {
			if s, ok := val.(string); ok {
				return s
			}
			return ""
		},
	)
	subCompiled, err := graph.Compile(subGraph, executor)
	if err != nil {
		t.Fatalf("Failed to compile subgraph: %v", err)
	}

	// Create subgraph node
	subgraphNode := graph.NewSubgraphNode(
		"processor",
		subCompiled,
		func(ctx context.Context, view state.ReadView) (string, error) {
			return "input", nil
		},
		func(ctx context.Context, output string) (state.Updates, error) {
			return state.Updates{"result": output}, nil
		},
		[]string{graph.EndNode},
	)

	// Verify we can add it to a builder
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}

	builder.AddNodeFunc("start", []string{"processor"},
		func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"processor"}, nil, nil
		},
	)
	builder.AddSubgraphNode(subgraphNode)
	builder.SetEntryPoint("start")

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile graph with subgraph: %v", err)
	}

	// Just verify it compiles successfully
	if compiled == nil {
		t.Error("Expected compiled graph to be non-nil")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
