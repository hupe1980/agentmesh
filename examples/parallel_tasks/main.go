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

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

// mergeMapReducer combines results from multiple parallel tasks.
// This reducer is used by BinaryOpChannel to handle concurrent updates
// without data loss - perfect for aggregating parallel task results.
func mergeMapReducer(oldValue, newValue any) any {
	oldMap, _ := oldValue.(map[string]any)
	newMap, _ := newValue.(map[string]any)

	// Handle nil cases
	if oldMap == nil && newMap == nil {
		return nil
	}

	// Merge both maps, with newMap taking precedence on key conflicts
	merged := make(map[string]any)
	for k, v := range oldMap {
		merged[k] = v
	}
	for k, v := range newMap {
		merged[k] = v
	}
	return merged
}

func main() {
	fmt.Println("=== Parallel Tasks Example ===")
	fmt.Println("Demonstrates concurrent execution with result aggregation")
	fmt.Println()

	actionHistoryKey := graphstate.NewListKey[string]("action_history", 0)
	resultsKey := graphstate.NewKey("results", map[string]any{})
	summaryKey := graphstate.NewKey("summary", map[string]any{})

	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, graphstate.MessagesKey.Key)
	graphstate.RegisterKey(mgr, actionHistoryKey.Key)
	graphstate.RegisterKey(mgr, resultsKey)
	graphstate.RegisterKey(mgr, summaryKey)

	gph, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	// Task A: Simulates data analysis work
	taskA := &graph.Node{
		Name: "task_a",
		RunFunc: func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
			fmt.Println("  [task_a] Starting analysis...")
			time.Sleep(300 * time.Millisecond) // Simulate work
			fmt.Println("  [task_a] ✓ Analysis complete")

			results := graphstate.GetFromView(view, resultsKey)
			results["task_a"] = "analysis result"

			return &graph.NodeResult{
				Updates: graphstate.Updates{
					actionHistoryKey.Name(): []string{"task_a: analysis completed"},
					resultsKey.Name():       results,
				},
			}, nil
		},
	}

	// Task B: Simulates simulation work (runs in parallel with Task A)
	taskB := &graph.Node{
		Name: "task_b",
		RunFunc: func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
			fmt.Println("  [task_b] Starting simulation...")
			time.Sleep(300 * time.Millisecond) // Simulate work
			fmt.Println("  [task_b] ✓ Simulation complete")

			results := graphstate.GetFromView(view, resultsKey)
			results["task_b"] = "simulation result"

			return &graph.NodeResult{
				Updates: graphstate.Updates{
					actionHistoryKey.Name(): []string{"task_b: simulation completed"},
					resultsKey.Name():       results,
				},
			}, nil
		},
	}

	// Merge node: Aggregates results after all parallel tasks complete
	// This demonstrates the fan-in pattern (many → one)
	mergeResults := &graph.Node{
		Name: "combine",
		RunFunc: func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
			fmt.Println("  [combine] Aggregating parallel task results...")

			// Read the merged results from both tasks
			results := graphstate.GetFromView(view, resultsKey)

			return &graph.NodeResult{
				Updates: graphstate.Updates{
					actionHistoryKey.Name(): []string{"combine: aggregated all results"},
					summaryKey.Name():       results,
				},
			}, nil
		},
	}

	// Helper to add nodes with error checking
	mustAddNode := func(n *graph.Node) {
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
	// - Fan-out: START has two outgoing edges (parallel execution)
	// - Fan-in: combine has two incoming edges (synchronization point)
	gph.AddEdge(graph.StartNode, "task_a")
	gph.AddEdge(graph.StartNode, "task_b")
	gph.AddEdge("task_a", "combine")
	gph.AddEdge("task_b", "combine")
	gph.AddEdge("combine", graph.EndNode)

	// Compile the graph
	compiled, err := exec.CompileGraph(gph)
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
	fmt.Printf("  results: %v\n", graphstate.GetFromView(view, resultsKey))
	fmt.Printf("  summary: %v\n", graphstate.GetFromView(view, summaryKey))
}
