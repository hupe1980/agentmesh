// Package main demonstrates parallel execution patterns in AgentMesh.
// This example shows how to:
//   - Execute independent tasks concurrently for improved performance
//   - Use BinaryOpChannel to merge results from parallel nodes
//   - Use TopicChannel to accumulate action history
//   - Control concurrency limits with MaxConcurrency option
//   - Implement fan-out (one node → many nodes) and fan-in (many nodes → one node) patterns
//
// Key concepts:
//   - Pregel BSP execution: Independent nodes in the same superstep run in parallel
//   - BinaryOpChannel: Custom reducer function merges concurrent updates
//   - Synchronization: All parallel nodes complete before the merge node executes
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	fmt.Println("=== Parallel Tasks Example ===")
	fmt.Println("Demonstrates concurrent execution with result aggregation")
	fmt.Println()

	actionHistoryKey := graphstate.NewListKey[string]("action_history", 0)
	resultAKey := graphstate.NewKey("result_a", "")
	resultBKey := graphstate.NewKey("result_b", "")
	summaryKey := graphstate.NewKey("summary", map[string]any{})

	mgr := graphstate.NewManager()
	// Register actionHistoryKey as a ListKey (TopicChannel)
	if err := graphstate.RegisterListKey(mgr, actionHistoryKey); err != nil {
		panic(fmt.Sprintf("Failed to register actionHistory key: %v", err))
	}
	if err := graphstate.RegisterKey(mgr, resultAKey); err != nil {
		panic(fmt.Sprintf("Failed to register resultA key: %v", err))
	}
	if err := graphstate.RegisterKey(mgr, resultBKey); err != nil {
		panic(fmt.Sprintf("Failed to register resultB key: %v", err))
	}
	if err := graphstate.RegisterKey(mgr, summaryKey); err != nil {
		panic(fmt.Sprintf("Failed to register summary key: %v", err))
	}

	gph, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	// Task A: Simulates data analysis (runs in parallel with Task B)
	taskA := &graph.BaseCommandNode{
		NodeName:        "task_a",
		DeclaredTargets: graph.NewTargetSet("combine"),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("  [task_a] Starting analysis...")
			time.Sleep(300 * time.Millisecond) // Simulate work
			fmt.Println("  [task_a] ✓ Analysis complete")

			builder := graphstate.NewUpdateBuilder()
			graphstate.AppendUpdate(builder, actionHistoryKey, "task_a: analysis completed")
			graphstate.SetUpdate(builder, resultAKey, "analysis result")
			updates, _ := builder.Build()
			return graph.Goto("combine", updates), nil
		},
	}

	// Task B: Simulates simulation work (runs in parallel with Task A)
	taskB := &graph.BaseCommandNode{
		NodeName:        "task_b",
		DeclaredTargets: graph.NewTargetSet("combine"),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("  [task_b] Starting simulation...")
			time.Sleep(300 * time.Millisecond) // Simulate work
			fmt.Println("  [task_b] ✓ Simulation complete")

			builder := graphstate.NewUpdateBuilder()
			graphstate.AppendUpdate(builder, actionHistoryKey, "task_b: simulation completed")
			graphstate.SetUpdate(builder, resultBKey, "simulation result")
			updates, _ := builder.Build()
			return graph.Goto("combine", updates), nil
		},
	}

	// Merge node: Aggregates results after all parallel tasks complete
	// This demonstrates the fan-in pattern (many → one)
	mergeResults := &graph.BaseCommandNode{
		NodeName:        "combine",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("  [combine] Aggregating parallel task results...")

			// Read results from both parallel tasks
			resultA := graphstate.GetFromView(view, resultAKey)
			resultB := graphstate.GetFromView(view, resultBKey)

			// Combine into summary map
			results := map[string]any{
				"task_a": resultA,
				"task_b": resultB,
			}

			builder := graphstate.NewUpdateBuilder()
			// graphstate.AppendUpdate(builder, actionHistoryKey, "combine: aggregated all results")
			graphstate.SetUpdate(builder, summaryKey, results)
			updates, _ := builder.Build()
			return graph.End(updates), nil
		},
	} // Helper to add nodes with error checking
	mustAddNode := func(n graph.Node) {
		if err := gph.AddNode(n); err != nil {
			panic(err)
		}
	}

	mustAddNode(taskA)
	mustAddNode(taskB)
	mustAddNode(mergeResults)

	// Build graph topology:
	//   START → task_a ↘
	//                   → combine → END
	//   START → task_b ↗
	//
	// This creates a fan-out/fan-in pattern where:
	// - Fan-out: START has multiple entry points (parallel execution)
	// - Fan-in: combine has two incoming edges (synchronization point)
	if err := gph.SetEntryPoint("task_a"); err != nil {
		panic(err)
	}
	if err := gph.SetEntryPoint("task_b"); err != nil {
		panic(err)
	}

	// Compile the graph
	compiled, err := graph.Compile(gph, graph.NewMessagePregelExecutor())
	if err != nil {
		fmt.Printf("❌ Compilation error: %v\n", err)
		return
	}

	// Execute with controlled concurrency
	// MaxConcurrency(2) allows both tasks to run simultaneously
	fmt.Println("Executing with max concurrency = 2")
	fmt.Println()

	started := time.Now()
	// Run the graph and consume all events (nodes don't produce messages, only state updates)
	for _, err := range compiled.Run(context.Background(), nil, graph.WithMaxConcurrency(2)) {
		if err != nil {
			fmt.Printf("❌ Execution error: %v\n", err)
			return
		}
	}
	elapsed := time.Since(started)

	fmt.Println()
	fmt.Printf("✓ Execution completed in %s\n", elapsed)
	fmt.Println("  (Note: Parallel execution is ~2x faster than sequential)")
	fmt.Println()
	fmt.Println("Final state:")
	ctx := context.Background()
	view, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  action_history: %v\n", graphstate.GetFromView(view, actionHistoryKey.Key))
	fmt.Printf("  result_a: %v\n", graphstate.GetFromView(view, resultAKey))
	fmt.Printf("  result_b: %v\n", graphstate.GetFromView(view, resultBKey))
	fmt.Printf("  summary: %v\n", graphstate.GetFromView(view, summaryKey))
}
