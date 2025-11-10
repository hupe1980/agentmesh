// Package main demonstrates checkpointing.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

func main() {
	ctx := context.Background()
	fmt.Println("=== Checkpoint Demo ===")
	runDemo(ctx)
}

func runDemo(ctx context.Context) {
	runID := "demo-workflow-123"
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	fmt.Println("This demo shows:")
	fmt.Println("1. Automatic checkpoint saving after each superstep")
	fmt.Println("2. Viewing checkpoint history")
	fmt.Println("3. Time-travel debugging (loading past states)")
	fmt.Println("4. Resuming from a checkpoint (simulated failure recovery)")
	fmt.Println()

	// === Part 1: Run workflow with automatic checkpointing ===
	fmt.Println("=== Part 1: Workflow with Automatic Checkpointing ===")
	fmt.Printf("Run ID: %s\n", runID)
	fmt.Println("Checkpoints will be saved automatically after every superstep")
	fmt.Println()

	compiled := buildWorkflow()

	seq := compiled.Run(ctx, nil,
		graph.WithRunID(runID),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1, // Save after every superstep
			AutoRestore:  false,
		}))

	fmt.Println("Workflow Results:")
	for event, err := range seq {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}
		fmt.Printf("Node: %s, Superstep: %d, Messages: %d\n", event.Node, compiled.CurrentSuperstep(), len(event.Messages))
	}

	// === Part 2: View checkpoint history ===
	fmt.Println("=== Part 2: Viewing Checkpoint History ===")
	checkpoints, err := checkpointer.List(ctx, runID)
	if err != nil {
		log.Fatalf("Failed to list: %v", err)
	}

	fmt.Printf("Found %d checkpoints (saved automatically):\n\n", len(checkpoints))
	for i, cp := range checkpoints {
		fmt.Printf("Checkpoint %d (Superstep %d):\n", i+1, cp.Superstep)
		fmt.Printf("  Time:      %v\n", cp.Timestamp.Format("15:04:05.000"))
		fmt.Printf("  Step:      %v\n", cp.State["step"])
		fmt.Printf("  Status:    %v\n", cp.State["status"])
		if data := cp.State["data"]; data != nil {
			fmt.Printf("  Data:      %v\n", data)
		}
		fmt.Printf("  Completed: %v\n", cp.CompletedNodes)
		fmt.Println()
	}

	// === Part 3: Time-travel debugging ===
	fmt.Println("=== Part 3: Time-Travel Debugging ===")
	fmt.Println("Loading checkpoint from superstep 2 to inspect past state...")
	fmt.Println()

	cp2, err := checkpointer.LoadAtSuperstep(ctx, runID, 2)
	if err != nil {
		log.Fatalf("Failed to load: %v", err)
	}

	if cp2 != nil {
		fmt.Println("✓ Time-traveled to superstep 2:")
		fmt.Printf("  Status at that time: %v\n", cp2.State["status"])
		fmt.Printf("  Data processed:      %v\n", cp2.State["data"])
		fmt.Printf("  Nodes completed:     %v\n", cp2.CompletedNodes)
		fmt.Println()
		fmt.Println("This is useful for:")
		fmt.Println("  • Debugging: See exact state when error occurred")
		fmt.Println("  • Auditing: Track what happened at each step")
		fmt.Println("  • Testing: Verify intermediate states")
	}
	fmt.Println()

	// === Part 4: Simulate failure and resume ===
	fmt.Println("=== Part 4: Failure Recovery (Resume from Checkpoint) ===")
	fmt.Println("Simulating a workflow that fails mid-execution...")
	fmt.Println()

	// Build a workflow that "fails" after step 2
	failingWorkflow := buildFailingWorkflow()
	failRunID := "failing-workflow"

	// First attempt - will fail
	failSeq := failingWorkflow.Run(ctx, nil,
		graph.WithRunID(failRunID),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1,
			AutoRestore:  false,
		}),
	)

	for _, err := range failSeq {
		if err != nil {
			fmt.Printf("❌ Workflow failed: %v\n", err)
			break
		}
	}
	fmt.Println()

	// Check what was saved
	failCheckpoints, _ := checkpointer.List(ctx, failRunID)
	fmt.Printf("Checkpoints saved before failure: %d\n", len(failCheckpoints))
	if len(failCheckpoints) > 0 {
		last := failCheckpoints[0]
		fmt.Printf("Last good state: Superstep %d, Status: %v\n", last.Superstep, last.State["status"])
	}
	fmt.Println()

	// Now resume with auto-restore
	fmt.Println("Resuming from last checkpoint with auto-restore enabled...")
	fixedWorkflow := buildFixedWorkflow()

	resumeSeq := fixedWorkflow.Run(ctx, nil,
		graph.WithRunID(failRunID),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1,
			AutoRestore:  true, // ← Automatically resume from last checkpoint
		}),
	)

	var lastErr error
	for _, err := range resumeSeq {
		if err != nil {
			lastErr = err
			log.Printf("Error: %v", err)
		}
	}

	if lastErr == nil {
		fmt.Println("✓ Workflow recovered and completed successfully!")
		fmt.Println("  • Restored state from last checkpoint")
		fmt.Println("  • Skipped already-completed nodes")
		fmt.Println("  • Continued from where it left off")
	}
	fmt.Println()

	// === Part 5: Statistics ===
	fmt.Println("=== Part 5: Checkpoint Statistics ===")
	stats := checkpointer.Stats()
	fmt.Printf("Total runs tracked: %d\n", len(stats))
	for rid, count := range stats {
		fmt.Printf("  %s: %d checkpoints\n", rid, count)
	}
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("Key Features Demonstrated:")
	fmt.Println("✓ Automatic checkpointing - no manual save() calls needed")
	fmt.Println("✓ Time-travel debugging - inspect any past superstep")
	fmt.Println("✓ Failure recovery - auto-resume from last checkpoint")
	fmt.Println("✓ Zero-copy safety - checkpoints are immutable snapshots")
	fmt.Println()
	fmt.Println("For production workflows:")
	fmt.Println("• Use SaveInterval to balance performance vs. recovery time")
	fmt.Println("• Consider persistent storage (SQLite, PostgreSQL) for durability")
	fmt.Println("• Set meaningful RunIDs for multi-user systems")
}

func buildWorkflow() *graph.Compiled {
	builder := graph.NewBuilder()

	builder.Node("step1", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("→ Step 1: Initializing...")
		time.Sleep(300 * time.Millisecond)
		return &graph.NodeResult{
			Updates: map[string]any{"step": 1, "status": "initialized"},
		}, nil
	})

	builder.Node("step2", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("→ Step 2: Processing data...")
		time.Sleep(300 * time.Millisecond)
		return &graph.NodeResult{
			Updates: map[string]any{"step": 2, "status": "processing", "data": []string{"A", "B", "C"}},
		}, nil
	})

	builder.Node("step3", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("→ Step 3: Finalizing...")
		time.Sleep(300 * time.Millisecond)
		return &graph.NodeResult{
			Updates: map[string]any{"step": 3, "status": "complete"},
		}, nil
	})

	builder.AddEdge(graph.StartNode, "step1")
	builder.AddEdge("step1", "step2")
	builder.AddEdge("step2", "step3")
	builder.AddEdge("step3", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}
	return compiled
}

func buildFailingWorkflow() *graph.Compiled {
	builder := graph.NewBuilder()

	builder.Node("step1", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("  Step 1: OK")
		return &graph.NodeResult{
			Updates: map[string]any{"step": 1, "status": "ok"},
		}, nil
	})

	builder.Node("step2", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("  Step 2: OK")
		return &graph.NodeResult{
			Updates: map[string]any{"step": 2, "status": "ok"},
		}, nil
	})

	builder.Node("step3", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		return nil, fmt.Errorf("simulated failure at step 3")
	})

	builder.AddEdge(graph.StartNode, "step1")
	builder.AddEdge("step1", "step2")
	builder.AddEdge("step2", "step3")
	builder.AddEdge("step3", graph.EndNode)

	compiled, _ := builder.Compile()
	return compiled
}

func buildFixedWorkflow() *graph.Compiled {
	builder := graph.NewBuilder()

	builder.Node("step1", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("  Step 1: Skipped (already completed)")
		return &graph.NodeResult{
			Updates: map[string]any{"step": 1, "status": "ok"},
		}, nil
	})

	builder.Node("step2", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("  Step 2: Skipped (already completed)")
		return &graph.NodeResult{
			Updates: map[string]any{"step": 2, "status": "ok"},
		}, nil
	})

	builder.Node("step3", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		fmt.Println("  Step 3: Now succeeding (bug fixed)")
		return &graph.NodeResult{
			Updates: map[string]any{"step": 3, "status": "fixed!"},
		}, nil
	})

	builder.AddEdge(graph.StartNode, "step1")
	builder.AddEdge("step1", "step2")
	builder.AddEdge("step2", "step3")
	builder.AddEdge("step3", graph.EndNode)

	compiled, _ := builder.Compile()
	return compiled
}
