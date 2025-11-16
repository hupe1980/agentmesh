// Package main demonstrates the Builder API for creating graphs.
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

func main() {
	// Create a builder with automatic compilation support
	builder, err := exec.NewBuilder()
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Build a simple workflow using fluent API
	builder.
		Node("analyze", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			fmt.Println("Analyzing input...")
			return &graph.NodeResult{
				Updates: map[string]any{
					"analysis": "Input looks good",
					"score":    0.95,
				},
			}, nil
		}).
		Node("validate", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			score := s.Get("score").(float64)
			fmt.Printf("Validating with score: %.2f\n", score)

			valid := score > 0.8
			return &graph.NodeResult{
				Updates: map[string]any{"valid": valid},
			}, nil
		}).
		Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			valid := s.Get("valid").(bool)
			if valid {
				fmt.Println("Processing validated input...")
				return &graph.NodeResult{
					Updates: map[string]any{"result": "Success!"},
				}, nil
			}
			return &graph.NodeResult{
				Updates: map[string]any{"result": "Failed validation"},
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

	// Get final state
	finalState := builder.StateManager()
	result := finalState.Get("result").(string)
	fmt.Printf("\nFinal result: %s\n", result)
}
