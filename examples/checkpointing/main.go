// Package main demonstrates checkpointing for fault tolerance and resume.
//
// Checkpointing enables:
//   - Save execution state at each superstep
//   - Resume from checkpoint after failure
//   - Time-travel debugging
//   - Audit trails of execution
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

// State keys for tracking workflow progress
var (
	stepKey   = graph.NewKey("step", 0)
	statusKey = graph.NewKey("status", "")
	dataKey   = graph.NewKey("data", "")
)

func main() {
	ctx := context.Background()
	fmt.Println("=== Checkpointing Example ===")

	// Example 1: Basic checkpointing with in-memory storage
	fmt.Println("\n--- Example 1: Basic Checkpointing ---")
	basicCheckpointingExample(ctx)

	// Example 2: Resume from checkpoint
	fmt.Println("\n--- Example 2: Resume from Checkpoint ---")
	resumeFromCheckpointExample(ctx)

	// Example 3: List and inspect checkpoints
	fmt.Println("\n--- Example 3: Checkpoint Inspection ---")
	inspectCheckpointsExample(ctx)
}

func basicCheckpointingExample(ctx context.Context) {
	// Create an in-memory checkpointer
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Build workflow graph
	g := graph.New[any, any](stepKey, statusKey, dataKey)

	g.Node("init", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		fmt.Println("  [init] Starting workflow...")
		return graph.Set(stepKey, 1).
			With(graph.SetValue(statusKey, "initialized")).
			To("process")
	}, "process")

	g.Node("process", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		step := graph.Get(scope, stepKey)
		fmt.Printf("  [process] Processing step %d...\n", step)
		return graph.Set(stepKey, step+1).
			With(graph.SetValue(statusKey, "processing")).
			With(graph.SetValue(dataKey, "processed-data")).
			To("validate")
	}, "validate")

	g.Node("validate", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		step := graph.Get(scope, stepKey)
		data := graph.Get(scope, dataKey)
		fmt.Printf("  [validate] Validating step %d, data=%s\n", step, data)
		return graph.Set(stepKey, step+1).
			With(graph.SetValue(statusKey, "validated")).
			To("complete")
	}, "complete")

	g.Node("complete", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		step := graph.Get(scope, stepKey)
		fmt.Printf("  [complete] Completing at step %d\n", step)
		return graph.Set(statusKey, "completed").End()
	}, graph.END)

	g.Start("init")

	// Configure with checkpointer
	g.WithCheckpointer(checkpointer, "run-001")

	compiled, err := g.Build()
	if err != nil {
		log.Fatalf("Failed to build: %v", err)
	}

	// Run workflow - checkpoints are saved at each superstep
	fmt.Println("\n  Running workflow with checkpointing enabled...")
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	fmt.Println("\n  ✓ Workflow completed with checkpoints saved!")

	// Show saved checkpoints using Stats()
	stats := checkpointer.Stats()
	fmt.Printf("  Saved runs: %d\n", len(stats))
	for runID, count := range stats {
		fmt.Printf("    - %s: %d checkpoints\n", runID, count)
	}
}

func resumeFromCheckpointExample(ctx context.Context) {
	// Create checkpointer and pre-populate with a "failed" run
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Simulate a partially completed run by saving a checkpoint
	partialCheckpoint := &checkpoint.Checkpoint{
		RunID:     "resume-run-001",
		Superstep: 2,
		State: map[string]any{
			"step":   2,
			"status": "processing",
			"data":   "partial-data",
		},
		CompletedNodes: []string{"init", "process"},
	}
	if err := checkpointer.Save(ctx, partialCheckpoint); err != nil {
		log.Fatalf("Failed to save partial checkpoint: %v", err)
	}
	fmt.Println("  Simulated partial run with checkpoint at superstep 2")

	// Build graph that can resume
	g := graph.New[any, any](stepKey, statusKey, dataKey)

	g.Node("init", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		fmt.Println("  [init] Would run if starting fresh")
		return graph.Set(stepKey, 1).To("process")
	}, "process")

	g.Node("process", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		fmt.Println("  [process] Would run if starting fresh")
		return graph.Set(stepKey, 2).To("validate")
	}, "validate")

	g.Node("validate", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		step := graph.Get(scope, stepKey)
		fmt.Printf("  [validate] Resuming! step=%d\n", step)
		return graph.Set(stepKey, step+1).
			With(graph.SetValue(statusKey, "validated")).
			To("complete")
	}, "complete")

	g.Node("complete", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		step := graph.Get(scope, stepKey)
		fmt.Printf("  [complete] Finishing resumed run, step=%d\n", step)
		return graph.Set(statusKey, "completed").End()
	}, graph.END)

	g.Start("init")
	g.WithCheckpointer(checkpointer, "resume-run-001")

	compiled, err := g.Build()
	if err != nil {
		log.Fatalf("Failed to build: %v", err)
	}

	// Load checkpoint and resume
	cp, err := checkpointer.Load(ctx, "resume-run-001")
	if err != nil {
		log.Fatalf("Failed to load checkpoint: %v", err)
	}

	fmt.Printf("  Loaded checkpoint: superstep=%d, completed=%v\n",
		cp.Superstep, cp.CompletedNodes)

	// Resume execution from checkpoint
	fmt.Println("\n  Resuming from checkpoint...")
	for _, err := range compiled.Run(ctx, nil, graph.WithCheckpoint(cp)) {
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	fmt.Println("\n  ✓ Resumed and completed successfully!")
}

func inspectCheckpointsExample(ctx context.Context) {
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Save a few checkpoints for different runs
	for i := 1; i <= 3; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     fmt.Sprintf("inspect-run-%03d", i),
			Superstep: int64(i * 2),
			State: map[string]any{
				"step":   i * 2,
				"status": fmt.Sprintf("step-%d-complete", i*2),
			},
		}
		if err := checkpointer.Save(ctx, cp); err != nil {
			log.Fatalf("Failed to save: %v", err)
		}
	}

	// List all runs using Stats()
	stats := checkpointer.Stats()

	fmt.Printf("  Found %d saved runs:\n", len(stats))
	for runID, count := range stats {
		cp, err := checkpointer.Load(ctx, runID)
		if err != nil {
			continue
		}
		fmt.Printf("    - %s: %d checkpoints, latest superstep=%d, state=%v\n",
			runID, count, cp.Superstep, cp.State)
	}

	fmt.Println("\n  Checkpointing enables:")
	fmt.Println("    • Fault tolerance - resume after crashes")
	fmt.Println("    • Time-travel debugging - inspect any state")
	fmt.Println("    • Audit trails - track execution history")
	fmt.Println("    • Long-running workflows - survive restarts")
}
