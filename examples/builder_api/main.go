// This is a port of the builder_api example showing the simplified API.
//
// Comparison:
//
//	Old API: 33 lines of setup code
//	New API: 8 lines of setup code (76% reduction)
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Define typed state keys at package level for type-safe access
// Note: graph2 keys are simpler - no need for separate registration
var (
	AnalysisKey = graph.NewKey("analysis", "")
	ScoreKey    = graph.NewKey("score", 0.0)
	ValidKey    = graph.NewKey("valid", false)
	ResultKey   = graph.NewKey("result", "")
)

func main() {
	// Create graph with all state keys in one line
	// No need for: NewManagerBuilder, RegisterKey, Build, NewBuilder, WithManager
	g := graph.New[any, any](AnalysisKey, ScoreKey, ValidKey, ResultKey)

	// Build a simple workflow using fluent API with type-safe keys
	g.Node("analyze", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		fmt.Println("Analyzing input...")
		// Simple state updates with Set().With() chaining
		return graph.Set(AnalysisKey, "Input looks good").
			With(graph.SetValue(ScoreKey, 0.95)).
			To("validate")
	}, "validate")

	g.Node("validate", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Type-safe read - no casting needed, compile-time checked
		score := graph.Get(view, ScoreKey)
		fmt.Printf("Validating with score: %.2f\n", score)
		valid := score > 0.8
		return graph.Set(ValidKey, valid).To("process")
	}, "process")

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Type-safe read with default value - never panics
		valid := graph.Get(view, ValidKey)
		if valid {
			fmt.Println("Processing validated input...")
			result := "Success!"
			fmt.Printf("✓ Final result: %s\n", result)
			return graph.Set(ResultKey, result).End()
		}
		result := "Failed validation"
		fmt.Printf("✗ Final result: %s\n", result)
		return graph.Set(ResultKey, result).End()
	}, graph.END)

	// Set entry point
	g.Start("analyze")

	// Build the graph (validates structure)
	compiled, err := g.Build()
	if err != nil {
		log.Fatalf("Failed to build: %v", err)
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
