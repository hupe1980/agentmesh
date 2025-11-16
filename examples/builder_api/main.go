// Package main demonstrates the Builder API for creating graphs with type-safe state keys.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Define typed state keys at package level for type safety and autocomplete
var (
	AnalysisKey = state.NewKey[string]("analysis")
	ScoreKey    = state.NewKey[float64]("score")
	ValidKey    = state.NewKey[bool]("valid")
	ResultKey   = state.NewKey[string]("result")
)

func main() {
	// Create a builder with automatic compilation support
	builder, err := exec.NewBuilder()
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Build a simple workflow using fluent API with type-safe keys
	builder.
		Node("analyze", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			fmt.Println("Analyzing input...")
			return &graph.NodeResult{
				Updates: map[string]any{
					AnalysisKey.Name(): "Input looks good",
					ScoreKey.Name():    0.95,
				},
			}, nil
		}).
		Node("validate", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			// Type-safe read - no casting needed, compile-time checked
			score, err := ScoreKey.Get(s)
			if err != nil {
				return nil, fmt.Errorf("failed to get score: %w", err)
			}
			fmt.Printf("Validating with score: %.2f\n", score)

			valid := score > 0.8
			return &graph.NodeResult{
				Updates: map[string]any{ValidKey.Name(): valid},
			}, nil
		}).
		Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			// Type-safe read with default value - never panics
			valid := ValidKey.GetOr(s, false)
			if valid {
				fmt.Println("Processing validated input...")
				return &graph.NodeResult{
					Updates: map[string]any{ResultKey.Name(): "Success!"},
				}, nil
			}
			return &graph.NodeResult{
				Updates: map[string]any{ResultKey.Name(): "Failed validation"},
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

	// Get final state with type safety - no casting, compile-time checked
	finalState := builder.StateManager()
	result, err := ResultKey.Get(finalState)
	if err != nil {
		log.Fatalf("Failed to get result: %v", err)
	}
	fmt.Printf("\nFinal result: %s\n", result)
}
