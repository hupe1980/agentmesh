// Package main demonstrates pausing workflow for human input.
//
// This example shows how to:
//   - Pause execution at a specific point for human input
//   - Save state with checkpointing
//   - Resume execution with updated state
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

var (
	questionKey = graph.NewKey("question", "")
	answerKey   = graph.NewKey("answer", "")
)

func main() {
	ctx := context.Background()
	fmt.Println("=== Human Pause Example ===")
	fmt.Println("  Demonstrates pausing for human input and resuming")
	fmt.Println()

	// Create checkpointer for state persistence
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "pause-run-001"

	// Build graph that pauses for human input
	g := graph.New[any, any](questionKey, answerKey)

	g.Node("ask", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		fmt.Println("  [ask] Preparing question for human...")
		return graph.Set(questionKey, "What is the capital of France?").To("wait_for_answer")
	}, "wait_for_answer")

	g.Node("wait_for_answer", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		answer := graph.Get(view, answerKey)
		fmt.Printf("  [wait_for_answer] Answer received: %s\n", answer)
		return graph.Cmd().To("process_answer")
	}, "process_answer")

	g.Node("process_answer", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		answer := graph.Get(view, answerKey)
		fmt.Printf("  [process_answer] Processing answer: %s\n", answer)
		if answer == "Paris" {
			fmt.Println("  [process_answer] ✓ Correct!")
		} else {
			fmt.Println("  [process_answer] ✗ Incorrect, but that's okay!")
		}
		return graph.Cmd().End()
	}, graph.END)

	g.Start("ask")

	// Interrupt before wait_for_answer to pause for human input
	g.InterruptBefore("wait_for_answer")

	// Configure checkpointer
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	// First run - will pause at the interrupt point
	fmt.Println("--- First Run (will pause for input) ---")
	for _, err := range compiled.Run(ctx, nil, graph.WithRunID(runID)) {
		if err != nil {
			var intErr *graph.InterruptError
			if errors.As(err, &intErr) {
				fmt.Printf("\n  ⏸️  Paused before: %s\n", intErr.NodeName)
				fmt.Println("     Checkpoint saved automatically")
				break
			}
			log.Fatal(err)
		}
	}

	// Simulate human providing input
	fmt.Println("\n  [Human provides answer: 'Paris']")

	// Load checkpoint and resume with human input
	fmt.Println("\n--- Resuming with Human Input ---")

	// Load the saved checkpoint
	savedCheckpoint, err := checkpointer.Load(ctx, runID)
	if err != nil {
		log.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Resume execution with WithResumeValue - sets state and auto-approves
	for _, err := range compiled.Run(ctx, nil,
		graph.WithRunID(runID),
		graph.WithCheckpoint(savedCheckpoint),
		graph.WithResumeValue("wait_for_answer", answerKey.Name(), "Paris"),
	) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("\n  ✓ Workflow completed!")
	fmt.Println()
	fmt.Println("  Human pause pattern:")
	fmt.Println("    1. InterruptBefore(node) - pause before a node")
	fmt.Println("    2. Checkpoint saved automatically")
	fmt.Println("    3. WithResumeValue(node, key, value) - inject input & auto-approve")
	fmt.Println("    4. WithCheckpoint(cp) - resume from saved state")
}
