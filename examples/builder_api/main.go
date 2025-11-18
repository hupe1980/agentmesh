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
	// Create state and register keys
	mgr := state.NewManager()
	state.RegisterKey(mgr, AnalysisKey)
	state.RegisterKey(mgr, ScoreKey)
	state.RegisterKey(mgr, ValidKey)
	state.RegisterKey(mgr, ResultKey)

	// Create a builder with the state
	builder, err := graph.NewBuilder(graph.WithManager(mgr))
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Build a simple workflow using fluent API with type-safe keys
	builder.
		Node("analyze", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			fmt.Println("Analyzing input...")
			return &graph.NodeResult{
				Updates: state.Updates{
					AnalysisKey.Name(): "Input looks good",
					ScoreKey.Name():    0.95,
				},
			}, nil
		}).
		Node("validate", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			// Type-safe read - no casting needed, compile-time checked
			score := state.GetFromView(view, ScoreKey)
			fmt.Printf("Validating with score: %.2f\n", score)

			valid := score > 0.8
			return &graph.NodeResult{
				Updates: state.Updates{ValidKey.Name(): valid},
			}, nil
		}).
		Node("process", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			// Type-safe read with default value - never panics
			valid := state.GetFromView(view, ValidKey)
			if valid {
				fmt.Println("Processing validated input...")
				return &graph.NodeResult{
					Updates: state.Updates{ResultKey.Name(): "Success!"},
				}, nil
			}
			return &graph.NodeResult{
				Updates: state.Updates{ResultKey.Name(): "Failed validation"},
			}, nil
		}).
		AddEdge(graph.StartNode, "analyze").
		AddEdge("analyze", "validate").
		AddEdge("validate", "process").
		AddEdge("process", graph.EndNode)

	// Compile the graph
	compiled, err := builder.Compile()
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}

	// Run the graph
	ctx := context.Background()
	messages := []message.Message{
		message.NewHumanMessageFromText("Hello, world!"),
	}

	fmt.Println("Running workflow...")
	for range compiled.Run(ctx, messages) {
	}

	// Get final state with type safety
	finalView, err := mgr.CreateReadView(ctx)
	if err != nil {
		log.Fatalf("Failed to create read view: %v", err)
	}
	result := state.GetFromView(finalView, ResultKey)
	fmt.Printf("\nFinal result: %s\n", result)
}
