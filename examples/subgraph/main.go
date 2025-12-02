// Package main demonstrates subgraph composition using graph.Subgraph().
//
// This example shows how to:
//   - Create reusable subgraphs with their own state
//   - Compose subgraphs into parent graphs using graph.Subgraph()
//   - Map state between parent and child graphs
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Parent graph keys
var (
	inputKey  = graph.NewKey("input", "")
	resultKey = graph.NewKey("result", "")
	stepsKey  = graph.NewListKey[string]("steps")
)

// Subgraph keys (isolated state)
var (
	subInputKey  = graph.NewKey("sub_input", "")
	subOutputKey = graph.NewKey("sub_output", "")
)

// createValidationSubgraph creates a reusable validation subgraph
func createValidationSubgraph() *graph.Graph[string, string] {
	g := graph.New[string, string](subInputKey, subOutputKey)

	g.Node("validate_format", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		input := graph.Get(view, subInputKey)
		fmt.Printf("    [validate] Checking format of: %s\n", input)
		if strings.TrimSpace(input) == "" {
			return graph.Fail(fmt.Errorf("empty input"))
		}
		return graph.To("validate_content")
	}, "validate_content")

	g.Node("validate_content", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		input := graph.Get(view, subInputKey)
		fmt.Printf("    [validate] Checking content of: %s\n", input)
		// Validation passed - set output
		return graph.Set(subOutputKey, "validated:"+input).End()
	}, graph.END)

	g.Start("validate_format")

	return g
}

// createTransformSubgraph creates a reusable transformation subgraph
func createTransformSubgraph() *graph.Graph[string, string] {
	g := graph.New[string, string](subInputKey, subOutputKey)

	g.Node("normalize", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		input := graph.Get(view, subInputKey)
		normalized := strings.ToLower(strings.TrimSpace(input))
		fmt.Printf("    [transform] Normalized: %s\n", normalized)
		return graph.Set(subOutputKey, normalized).To("enrich")
	}, "enrich")

	g.Node("enrich", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		output := graph.Get(view, subOutputKey)
		enriched := fmt.Sprintf("enriched(%s)", output)
		fmt.Printf("    [transform] Enriched: %s\n", enriched)
		return graph.Set(subOutputKey, enriched).End()
	}, graph.END)

	g.Start("normalize")

	return g
}

func main() {
	ctx := context.Background()
	fmt.Println("=== Subgraph Composition Example ===")
	fmt.Println("  Demonstrates graph.Subgraph() for composing reusable graphs")
	fmt.Println()

	// Create reusable subgraphs
	validationSubgraph := createValidationSubgraph()
	transformSubgraph := createTransformSubgraph()

	// Build them once (they can be reused)
	if _, err := validationSubgraph.Build(); err != nil {
		log.Fatal(err)
	}
	if _, err := transformSubgraph.Build(); err != nil {
		log.Fatal(err)
	}

	// Build the main graph that orchestrates subgraphs
	g := graph.New[any, any](inputKey, resultKey, stepsKey)

	// Entry point
	g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		fmt.Println("  [start] Beginning workflow")
		return graph.Set(inputKey, "  Raw Data  ").
			With(graph.AppendValue(stepsKey, "Started main workflow")).
			To("run_validation")
	}, "run_validation")

	// Use graph.Subgraph() to embed the validation subgraph
	g.Node("run_validation", graph.Subgraph(
		validationSubgraph,
		// Input mapper: parent state -> subgraph input
		func(ctx context.Context, view graph.View) (string, error) {
			input := graph.Get(view, inputKey)
			fmt.Printf("  [parent] Mapping input to validation subgraph: %s\n", input)
			return input, nil
		},
		// Output mapper: subgraph output -> parent state updates
		func(ctx context.Context, output string) (graph.Updates, error) {
			fmt.Printf("  [parent] Got validation result: %s\n", output)
			return graph.Updates{
				inputKey.Name(): output,
				stepsKey.Name(): graph.SliceOf[string]([]string{"Validation completed"}),
			}, nil
		},
	), "run_transform")

	// Use graph.Subgraph() to embed the transformation subgraph
	g.Node("run_transform", graph.Subgraph(
		transformSubgraph,
		// Input mapper
		func(ctx context.Context, view graph.View) (string, error) {
			input := graph.Get(view, inputKey)
			fmt.Printf("  [parent] Mapping input to transform subgraph: %s\n", input)
			return input, nil
		},
		// Output mapper
		func(ctx context.Context, output string) (graph.Updates, error) {
			fmt.Printf("  [parent] Got transform result: %s\n", output)
			return graph.Updates{
				resultKey.Name(): output,
				stepsKey.Name():  graph.SliceOf[string]([]string{"Transform completed"}),
			}, nil
		},
	), "finalize")

	// Finalize and show results
	g.Node("finalize", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		result := graph.Get(view, resultKey)
		steps := graph.GetList(view, stepsKey)

		fmt.Println("\n  Workflow Summary:")
		fmt.Printf("    Final result: %s\n", result)
		fmt.Println("    Steps executed:")
		for i, step := range steps {
			fmt.Printf("      %d. %s\n", i+1, step)
		}
		return graph.To(graph.END)
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println()
	fmt.Println("  Subgraph features:")
	fmt.Println("    • graph.Subgraph(sub, inputMapper, outputMapper)")
	fmt.Println("    • Subgraphs have isolated state")
	fmt.Println("    • Input/output mappers bridge parent ↔ child state")
	fmt.Println("    • Subgraphs can be reused across multiple nodes")
}
