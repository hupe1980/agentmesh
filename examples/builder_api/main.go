// Package main demonstrates the Builder API for creating graphs with type-safe state keys.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Define typed state keys at package level for type safety and autocomplete
var (
	AnalysisKey = state.NewKey("analysis", "")
	ScoreKey    = state.NewKey("score", 0.0)
	ValidKey    = state.NewKey("valid", false)
	ResultKey   = state.NewKey("result", "")
)

func main() {
	// Create a builder using graph.NewBuilder with Pregel executor
	// This uses the Pregel executor - perfect for parallel state transformations
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Register state keys with the builder's manager
	mgr := builder.Manager()
	state.RegisterKey(mgr, AnalysisKey)
	state.RegisterKey(mgr, ScoreKey)
	state.RegisterKey(mgr, ValidKey)
	state.RegisterKey(mgr, ResultKey)

	// Build a simple workflow using fluent API with type-safe keys
	builder.
		AddNodeFunc("analyze", []string{"validate"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			fmt.Println("Analyzing input...")
			updates := state.Updates{}
			updates[AnalysisKey.Name()] = "Input looks good"
			updates[ScoreKey.Name()] = 0.95
			return []string{"validate"}, updates, nil
		}).
		AddNodeFunc("validate", []string{"process"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Type-safe read - no casting needed, compile-time checked
			score := state.GetFromView(view, ScoreKey)
			fmt.Printf("Validating with score: %.2f\n", score)

			valid := score > 0.8
			updates := state.Updates{}
			updates[ValidKey.Name()] = valid
			return []string{"process"}, updates, nil
		}).
		AddNodeFunc("process", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Type-safe read with default value - never panics
			valid := state.GetFromView(view, ValidKey)
			if valid {
				fmt.Println("Processing validated input...")
				result := "Success!"
				fmt.Printf("✓ Final result: %s\n", result)
				updates := state.Updates{}
				updates[ResultKey.Name()] = result
				return []string{graph.EndNode}, updates, nil
			}
			result := "Failed validation"
			fmt.Printf("✗ Final result: %s\n", result)
			updates := state.Updates{}
			updates[ResultKey.Name()] = result
			return []string{graph.EndNode}, updates, nil
		}).
		SetEntryPoint("analyze")

	// Compile the graph
	compiled, err := builder.Compile()
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}

	// Run the graph
	ctx := context.Background()

	fmt.Println("Running workflow...")
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatalf("Execution error: %v", err)
		}
	}

	fmt.Println("\n✓ Workflow completed successfully")
}
