// Package main demonstrates human-in-the-loop approval workflow.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

var (
	taskKey   = graph.NewKey[string]("task")
	statusKey = graph.NewKey[string]("status")
)

func main() {
	ctx := context.Background()
	fmt.Println("=== Human Approval Example ===")

	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "approval-workflow-001"

	// Build graph with approval checkpoint
	g := graph.New[any, any](taskKey, statusKey)

	g.Node("prepare", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		fmt.Println("  [prepare] Preparing task for approval")
		return graph.Set(taskKey, "delete-production-database").
			With(graph.SetValue(statusKey, "awaiting_approval")).
			To("approve")
	}, "approve")

	// Node that requires approval before execution
	g.Node("approve", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		task := graph.Get(scope, taskKey)
		fmt.Printf("  [approve] Processing approved task: %s\n", task)
		return graph.Set(statusKey, "approved").To("execute")
	}, "execute")

	g.Node("execute", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		task := graph.Get(scope, taskKey)
		fmt.Printf("  [execute] Executing: %s\n", task)
		return graph.Set(statusKey, "completed").End()
	}, graph.END)

	g.Start("prepare")

	// Add interrupt before the approve node
	g.InterruptBefore("approve")

	// Configure checkpointer for resume support
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	// First run - will interrupt at approval
	fmt.Println("\n--- First Run (will pause for approval) ---")
	for _, err := range compiled.Run(ctx, nil, graph.WithRunID(runID)) {
		if err != nil {
			var intErr *graph.InterruptError
			if errors.As(err, &intErr) {
				fmt.Printf("  [INTERRUPT] Paused before node: %s\n", intErr.NodeName)
				fmt.Println("  Simulating human review...")
			} else {
				log.Fatal(err)
			}
		}
	}

	// Resume with approval
	fmt.Println("\n--- Resume with Approval ---")
	approval := &graph.ApprovalResponse{
		Decision:  graph.ApprovalApproved,
		Reason:    "Approved by admin",
		User:      "admin@example.com",
		Timestamp: time.Now(),
	}
	for _, err := range compiled.Resume(ctx, runID, graph.WithApproval("approve", approval)) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("\n  Human approval workflow enables:")
	fmt.Println("    • Pausing before sensitive operations")
	fmt.Println("    • Human review of planned actions")
	fmt.Println("    • Audit trail of approvals")
	fmt.Println("    • Resume/reject capability")
}
