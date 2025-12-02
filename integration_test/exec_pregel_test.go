package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

func TestPregelExecutor(t *testing.T) {
	// Define typed keys
	startedKey := graph.NewKey("started", false)
	task1Key := graph.NewKey("task1", "")
	task2Key := graph.NewKey("task2", "")
	completedKey := graph.NewKey("completed", false)

	var counter atomic.Int32

	// Build a graph with parallel nodes using new API
	g := graph.New[any, any](startedKey, task1Key, task2Key, completedKey)

	g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		counter.Add(1)
		return graph.Set(startedKey, true).To("task1", "task2")
	}, "task1", "task2")

	// Two nodes that can run in parallel
	g.Node("task1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		counter.Add(1)
		return graph.Set(task1Key, "done").To("end")
	}, "end")

	g.Node("task2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		counter.Add(1)
		return graph.Set(task2Key, "done").To("end")
	}, "end")

	g.Node("end", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		counter.Add(1)
		return graph.Set(completedKey, true).End()
	}, graph.END)

	g.Start("start")

	// Compile and execute
	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx := context.Background()

	resultCount := 0
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
		resultCount++
	}

	// Verify all nodes executed (note: "end" may execute multiple times due to parallel routing)
	if counter.Load() < 4 {
		t.Errorf("Expected at least 4 nodes to execute, got %d", counter.Load())
	}

	t.Logf("✅ Pregel executor test passed! Executed %d nodes", counter.Load())
}
