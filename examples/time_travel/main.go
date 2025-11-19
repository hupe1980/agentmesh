// Package main demonstrates time travel.

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

// This example demonstrates time-travel debugging using the checkpoint API.
// You can inspect historical state at any superstep,
// and compare different execution runs.

func main() {
	fmt.Println("=== Time-Travel Debugging Example ===")
	fmt.Println()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	valueKey := graphstate.NewKey("value", 0)

	// Build a simple mathematical workflow
	buildWorkflow := func(initialValue int) *graph.Compiled[[]message.Message, message.Message] {
		mgr := graphstate.NewManager()
		graphstate.RegisterKey(mgr, agent.MessagesKey.Key)
		graphstate.RegisterKey(mgr, valueKey)
		if err := mgr.ApplyUpdates(context.Background(), graphstate.Updates{
			valueKey.Name(): initialValue,
		}); err != nil {
			panic(err)
		}

		builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
		if err != nil {
			panic(err)
		}

		// Step 1: Double the value
		builder.Node("double", func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			value := graphstate.GetFromView(view, valueKey)
			newValue := value * 2
			fmt.Printf("  [double] %d → %d\n", value, newValue)
			return graphstate.Updates{valueKey.Name(): newValue}, nil
		})

		// Step 2: Add 10
		builder.Node("add_ten", func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			value := graphstate.GetFromView(view, valueKey)
			newValue := value + 10
			fmt.Printf("  [add_ten] %d → %d\n", value, newValue)
			return graphstate.Updates{valueKey.Name(): newValue}, nil
		})

		// Step 3: Multiply by 3
		builder.Node("multiply_three", func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			value := graphstate.GetFromView(view, valueKey)
			newValue := value * 3
			fmt.Printf("  [multiply_three] %d → %d\n", value, newValue)
			return graphstate.Updates{valueKey.Name(): newValue}, nil
		})

		builder.AddEdge(graph.StartNode, "double")
		builder.AddEdge("double", "add_ten")
		builder.AddEdge("add_ten", "multiply_three")
		builder.AddEdge("multiply_three", graph.EndNode)

		compiled, err := builder.Compile()
		if err != nil {
			log.Fatal(err)
		}
		return compiled
	}

	// ===== RUN 1: Starting with value = 1 =====
	fmt.Println("Run 1: Starting with value = 1")
	compiled := buildWorkflow(1)
	runID1 := "run-1"

	seq := compiled.Run(ctx, nil,
		graph.WithRunID(runID1),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1, // Save after every superstep
			AutoRestore:  false,
		}),
	)

	for _, err := range seq {
		if err != nil {
			log.Fatalf("Run 1 failed: %v", err)
		}
	}
	view1, err := compiled.Manager().CreateReadView(context.Background())
	if err != nil {
		log.Fatalf("Failed to create read view: %v", err)
	}
	result1 := graphstate.GetFromView(view1, valueKey)
	fmt.Printf("  Final value: %d\n\n", result1)

	// ===== RUN 2: Starting with value = 5 =====
	fmt.Println("Run 2: Starting with value = 5")
	compiled = buildWorkflow(5)
	runID2 := "run-2"

	seq2 := compiled.Run(ctx, nil,
		graph.WithRunID(runID2),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1,
			AutoRestore:  false,
		}),
	)

	for _, err := range seq2 {
		if err != nil {
			log.Fatal(err)
		}
	}

	view2, err := compiled.Manager().CreateReadView(context.Background())
	if err != nil {
		log.Fatalf("Failed to create read view: %v", err)
	}
	result2 := graphstate.GetFromView(view2, valueKey)
	fmt.Printf("  Final value: %d\n\n", result2)

	// ===== RUN 3: Starting with value = 10 =====
	fmt.Println("Run 3: Starting with value = 10")
	compiled = buildWorkflow(10)
	runID3 := "run-3"

	seq3 := compiled.Run(ctx, nil,
		graph.WithRunID(runID3),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1,
			AutoRestore:  false,
		}),
	)

	for _, err := range seq3 {
		if err != nil {
			log.Fatal(err)
		}
	}

	view3, err := compiled.Manager().CreateReadView(context.Background())
	if err != nil {
		log.Fatalf("Failed to create read view: %v", err)
	}
	result3 := graphstate.GetFromView(view3, valueKey)
	fmt.Printf("  Final value: %d\n\n", result3)

	// ===== TIME-TRAVEL DEBUGGING =====
	fmt.Println("=== Time-Travel Debugging Features ===")
	fmt.Println()

	// 1. List all saved checkpoints
	fmt.Println("1. Listing all saved runs:")
	allRuns := []string{runID1, runID2, runID3}
	for _, runID := range allRuns {
		checkpoints, err := checkpointer.List(ctx, runID)
		if err != nil {
			log.Fatal(err)
		}
		if len(checkpoints) > 0 {
			fmt.Printf("  Run: %s (%d checkpoints)\n", runID, len(checkpoints))
			for _, cp := range checkpoints {
				value := cp.State["value"]
				fmt.Printf("    - Superstep %d: value = %v (time: %s)\n",
					cp.Superstep, value, cp.Timestamp.Format("15:04:05.000"))
			}
		}
	}
	fmt.Println()

	// 2. Load a specific checkpoint (time-travel)
	fmt.Println("2. Time-travel: Loading checkpoint at superstep 2 from Run 1:")
	cp, err := checkpointer.LoadAtSuperstep(ctx, runID1, 2)
	if err != nil {
		log.Fatal(err)
	}
	if cp != nil {
		fmt.Printf("  Value at superstep 2: %v\n", cp.State["value"])
		fmt.Printf("  Completed nodes: %v\n", cp.CompletedNodes)
		fmt.Printf("  Timestamp: %s\n", cp.Timestamp.Format("15:04:05.000"))
		fmt.Println("  → This shows the exact state after the 'add_ten' node")
	}
	fmt.Println()

	// 3. Compare intermediate states across runs
	fmt.Println("3. Comparing intermediate states across all runs:")
	fmt.Println("  At superstep 2 (after double + add_ten):")
	for _, runID := range allRuns {
		cp, err := checkpointer.LoadAtSuperstep(ctx, runID, 2)
		if err == nil && cp != nil {
			fmt.Printf("    %s: value = %v\n", runID, cp.State["value"])
		}
	}
	fmt.Println()

	// 4. Show the complete history for one run
	fmt.Println("4. Complete execution history for Run 3:")
	checkpoints, _ := checkpointer.List(ctx, runID3)
	fmt.Println("  Superstep | Value | Nodes Completed")
	fmt.Println("  --------- | ----- | ---------------")
	for _, cp := range checkpoints {
		fmt.Printf("  %-9d | %-5v | %v\n", cp.Superstep, cp.State["value"], cp.CompletedNodes)
	}
	fmt.Println()

	// 5. Compare final results
	fmt.Println("5. Comparing final results:")
	fmt.Printf("  Run 1 (started with 1):  Final = %d\n", result1)
	fmt.Printf("  Run 2 (started with 5):  Final = %d\n", result2)
	fmt.Printf("  Run 3 (started with 10): Final = %d\n", result3)
	fmt.Printf("  Difference (Run 2 - Run 1): %d\n", result2-result1)
	fmt.Println()

	// 6. Time-travel use case
	fmt.Println("6. Time-travel debugging use case:")
	fmt.Println("  Scenario: Run 3 produced unexpected result")
	fmt.Println("  Solution: Load checkpoints to see where it went wrong")
	fmt.Println()
	for superstep := int64(1); superstep <= 3; superstep++ {
		cp, err := checkpointer.LoadAtSuperstep(ctx, runID3, superstep)
		if err == nil && cp != nil {
			fmt.Printf("    Superstep %d: value = %v\n", superstep, cp.State["value"])
		}
	}
	fmt.Println("  → You can inspect any intermediate state without re-running")
	fmt.Println()

	// 7. Cleanup
	fmt.Println("7. Cleanup: Deleting Run 1")
	if err := checkpointer.Delete(ctx, runID1); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  ✓ Run 1 deleted")

	stats := checkpointer.Stats()
	fmt.Printf("  Remaining runs: %d\n", len(stats))
	fmt.Println()

	fmt.Println("=== Summary ===")
	fmt.Println("Time-travel debugging enables:")
	fmt.Println("✓ Inspecting state at any superstep without re-running")
	fmt.Println("✓ Comparing execution across different runs")
	fmt.Println("✓ Loading intermediate states for debugging")
	fmt.Println("✓ Zero-overhead checkpointing (automatic)")
	fmt.Println("✓ Production debugging with historical snapshots")
	fmt.Println()
	fmt.Println("Use cases:")
	fmt.Println("• Debug production failures by loading the exact failing state")
	fmt.Println("• Compare A/B test results at each step")
	fmt.Println("• Audit trail for compliance requirements")
	fmt.Println("• Identify divergence points in different runs")
}
