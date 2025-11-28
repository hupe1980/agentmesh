// Package main demonstrates the Builder API for creating graphs with type-safe state keys.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
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
	// Register state keys first
	stateBuilder := state.NewManagerBuilder()
	state.RegisterKey(stateBuilder, AnalysisKey)
	state.RegisterKey(stateBuilder, ScoreKey)
	state.RegisterKey(stateBuilder, ValidKey)
	state.RegisterKey(stateBuilder, ResultKey)
	mgr := stateBuilder.Build()

	// Create a builder using graph.NewBuilder with Pregel executor and pre-configured manager
	// This uses the Pregel executor - perfect for parallel state transformations
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Build a simple workflow using fluent API with type-safe keys
	builder.
		AddNodeFunc("analyze", []string{"validate"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			fmt.Println("Analyzing input...")
			// Use type-safe UpdateBuilder for state changes
			return []string{"validate"}, state.NewUpdateBuilder().
				With(state.SetValue(AnalysisKey, "Input looks good")).
				With(state.SetValue(ScoreKey, 0.95)).
				Build(), nil
		}).
		AddNodeFunc("validate", []string{"process"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Type-safe read - no casting needed, compile-time checked
			score := state.GetFromView(view, ScoreKey)
			fmt.Printf("Validating with score: %.2f\n", score)

			valid := score > 0.8
			return []string{"process"}, state.NewUpdateBuilder().
				With(state.SetValue(ValidKey, valid)).
				Build(), nil
		}).
		AddNodeFunc("process", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Type-safe read with default value - never panics
			valid := state.GetFromView(view, ValidKey)
			if valid {
				fmt.Println("Processing validated input...")
				result := "Success!"
				fmt.Printf("✓ Final result: %s\n", result)
				return []string{graph.EndNode}, state.NewUpdateBuilder().
					With(state.SetValue(ResultKey, result)).
					Build(), nil
			}
			result := "Failed validation"
			fmt.Printf("✗ Final result: %s\n", result)
			return []string{graph.EndNode}, state.NewUpdateBuilder().
				With(state.SetValue(ResultKey, result)).
				Build(), nil
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
