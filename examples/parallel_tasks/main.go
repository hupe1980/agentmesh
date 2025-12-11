// Package main demonstrates parallel execution patterns with graph.
// This example shows how to:
//   - Execute independent tasks concurrently for improved performance
//   - Use ListKey to accumulate action history
//   - Control concurrency limits with WithMaxConcurrency option
//   - Implement fan-out (one node -> many nodes) and fan-in (many nodes -> one node) patterns
//
// Key concepts:
//   - Pregel BSP execution: Independent nodes in the same superstep run in parallel
//   - Synchronization: All parallel nodes complete before the merge node executes
//
// Comparison with old API:
//   Old API: 65 lines of setup code
//   New API: 45 lines (30% reduction)
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Define typed keys at package level
var (
	ActionHistoryKey = graph.NewListKey[string]("action_history")
	ResultAKey       = graph.NewKey("result_a", "")
	ResultBKey       = graph.NewKey("result_b", "")
	SummaryKey       = graph.NewKey("summary", map[string]any{})
)

func main() {
	fmt.Println("=== Parallel Tasks Example (graph2) ===")
	fmt.Println("Demonstrates concurrent execution with result aggregation")
	fmt.Println()

	// Create graph with all keys - no manual registration needed
	g := graph.New[string, string](
		ActionHistoryKey,
		ResultAKey,
		ResultBKey,
		SummaryKey,
	)

	// Task A: Simulates data analysis (runs in parallel with Task B)
	g.Node("task_a", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		fmt.Println("  [task_a] Starting analysis...")
		time.Sleep(300 * time.Millisecond) // Simulate work
		fmt.Println("  [task_a] Analysis complete")

		return graph.Append(ActionHistoryKey, "task_a: analysis completed").
			With(graph.SetValue(ResultAKey, "analysis result")).
			To("combine")
	}, "combine")

	// Task B: Simulates simulation work (runs in parallel with Task A)
	g.Node("task_b", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		fmt.Println("  [task_b] Starting simulation...")
		time.Sleep(300 * time.Millisecond) // Simulate work
		fmt.Println("  [task_b] Simulation complete")

		return graph.Append(ActionHistoryKey, "task_b: simulation completed").
			With(graph.SetValue(ResultBKey, "simulation result")).
			To("combine")
	}, "combine")

	// Combine node: Aggregates results after all parallel tasks complete
	// This demonstrates the fan-in pattern (many -> one)
	g.Node("combine", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		fmt.Println("  [combine] Aggregating parallel task results...")

		// Read results from both parallel tasks - type-safe access
		resultA := graph.Get(scope, ResultAKey)
		resultB := graph.Get(scope, ResultBKey)

		// Combine into summary map
		summary := map[string]any{
			"task_a": resultA,
			"task_b": resultB,
		}

		return graph.Set(SummaryKey, summary).End()
	}, graph.END)

	// Build graph topology:
	//   START -> task_a ↘
	//                    -> combine -> END
	//   START -> task_b ↗
	//
	// Multiple entry points for parallel execution
	g.Start("task_a", "task_b")

	// Build the graph
	compiled, err := g.Build()
	if err != nil {
		fmt.Printf("Build error: %v\n", err)
		return
	}

	// Execute with controlled concurrency
	// WithMaxConcurrency(2) allows both tasks to run simultaneously
	fmt.Println("Executing with max concurrency = 2")
	fmt.Println()

	started := time.Now()
	ctx := context.Background()
	for _, err := range compiled.Run(ctx, "", graph.WithMaxConcurrency(2)) {
		if err != nil {
			fmt.Printf("Execution error: %v\n", err)
			return
		}
	}
	elapsed := time.Since(started)

	fmt.Println()
	fmt.Printf("Execution completed in %s\n", elapsed)
	fmt.Println("  (Note: Parallel execution is ~2x faster than sequential)")
}
