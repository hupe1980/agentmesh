// Package main demonstrates time-travel debugging with checkpoints.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

var (
	stepKey   = graph.NewKey[int]("step")
	valueKey  = graph.NewKey[int]("value")
	statusKey = graph.NewKey[string]("status")
)

func main() {
	ctx := context.Background()
	fmt.Println("=== Time Travel Debugging Example ===")

	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Build multi-step workflow
	g := graph.New(stepKey, valueKey, statusKey)

	g.Node("step1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("  [step1] Initial processing")
		return graph.Set(stepKey, 1).
			With(graph.SetValue(valueKey, 100)).
			With(graph.SetValue(statusKey, "step1-complete")).
			To("step2")
	}, "step2")

	g.Node("step2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		v := graph.Get(scope, valueKey)
		fmt.Printf("  [step2] Processing value=%d\n", v)
		return graph.Set(stepKey, 2).
			With(graph.SetValue(valueKey, v*2)).
			With(graph.SetValue(statusKey, "step2-complete")).
			To("step3")
	}, "step3")

	g.Node("step3", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		v := graph.Get(scope, valueKey)
		fmt.Printf("  [step3] Final value=%d\n", v)
		return graph.Set(stepKey, 3).
			With(graph.SetValue(statusKey, "complete")).
			End()
	}, graph.END)

	g.Start("step1")
	g.WithCheckpointer(checkpointer, "time-travel-run")

	compiled, _ := g.Build()

	// Run full workflow
	fmt.Println("\n--- Running Full Workflow ---")
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	// List all checkpoints (time travel history)
	fmt.Println("\n--- Time Travel History ---")
	checkpoints, _ := checkpointer.List(ctx, "time-travel-run")
	for _, cp := range checkpoints {
		fmt.Printf("  Superstep %d: step=%v, value=%v, status=%v\n",
			cp.Superstep, cp.State["step"], cp.State["value"], cp.State["status"])
	}

	// Load checkpoint at specific superstep
	fmt.Println("\n--- Time Travel to Superstep 1 ---")
	cp, err := checkpointer.LoadAtSuperstep(ctx, "time-travel-run", 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  State at superstep 1: step=%v, value=%v, status=%v\n",
		cp.State["step"], cp.State["value"], cp.State["status"])

	fmt.Println("\n  Time travel enables:")
	fmt.Println("    • Inspect state at any point in execution")
	fmt.Println("    • Debug issues by replaying from earlier state")
	fmt.Println("    • Compare state changes across supersteps")
}
