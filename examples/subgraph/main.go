// Package main demonstrates subgraph composition using graph.Subgraph().
//
// This example shows how to:
//   - Create reusable subgraphs with their own state
//   - Compose subgraphs into parent graphs using graph.Subgraph()
//   - Map messages between parent and child graphs
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Parent graph keys for tracking workflow state
var (
	stepsKey = graph.NewListKey[string]("steps")
)

// createValidationSubgraph creates a reusable validation subgraph
func createValidationSubgraph() *graph.Graph {
	g := graph.New(stepsKey)

	g.Node("validate_format", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Get the last message (input to validate)
		lastMsg := graph.LastMessage(scope)
		input := ""
		if lastMsg != nil {
			input = lastMsg.String()
		}
		fmt.Printf("    [validate] Checking format of: %s\n", input)
		if strings.TrimSpace(input) == "" {
			return graph.Fail(fmt.Errorf("empty input"))
		}
		return graph.To("validate_content")
	}, "validate_content")

	g.Node("validate_content", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		lastMsg := graph.LastMessage(scope)
		input := ""
		if lastMsg != nil {
			input = lastMsg.String()
		}
		fmt.Printf("    [validate] Checking content of: %s\n", input)
		// Validation passed - emit validated output
		validatedMsg := message.NewAIMessageFromText("validated:" + input)
		return graph.Reply(validatedMsg).End()
	}, graph.END)

	g.Start("validate_format")

	compiled, err := g.Build()
	if err != nil {
		panic(err)
	}

	return compiled
}

// createTransformSubgraph creates a reusable transformation subgraph
func createTransformSubgraph() *graph.Graph {
	g := graph.New(stepsKey)

	g.Node("normalize", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		lastMsg := graph.LastMessage(scope)
		input := ""
		if lastMsg != nil {
			input = lastMsg.String()
		}
		normalized := strings.ToLower(strings.TrimSpace(input))
		fmt.Printf("    [transform] Normalized: %s\n", normalized)
		// Store intermediate result and continue
		normalizedMsg := message.NewAIMessageFromText(normalized)
		return graph.Reply(normalizedMsg).To("enrich")
	}, "enrich")

	g.Node("enrich", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		lastMsg := graph.LastMessage(scope)
		output := ""
		if lastMsg != nil {
			output = lastMsg.String()
		}
		enriched := fmt.Sprintf("enriched(%s)", output)
		fmt.Printf("    [transform] Enriched: %s\n", enriched)
		enrichedMsg := message.NewAIMessageFromText(enriched)
		return graph.Reply(enrichedMsg).End()
	}, graph.END)

	g.Start("normalize")

	compiled, err := g.Build()
	if err != nil {
		panic(err)
	}

	return compiled
}

func main() {
	ctx := context.Background()
	fmt.Println("=== Subgraph Composition Example ===")
	fmt.Println("  Demonstrates graph.Subgraph() for composing reusable graphs")
	fmt.Println()

	// Create reusable subgraphs (already compiled)
	validationSubgraph := createValidationSubgraph()
	transformSubgraph := createTransformSubgraph()

	// Build the main graph that orchestrates subgraphs
	g := graph.New(stepsKey)

	// Entry point
	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("  [start] Beginning workflow")
		// Emit initial input as a message
		inputMsg := message.NewHumanMessageFromText("  Raw Data  ")
		return graph.Reply(inputMsg).
			With(graph.SetValue(stepsKey, []string{"Started main workflow"})).
			To("run_validation")
	}, "run_validation")

	// Use graph.Subgraph() to embed the validation subgraph
	g.Node("run_validation", graph.Subgraph(
		validationSubgraph,
		// Input mapper: parent messages -> subgraph input messages
		func(ctx context.Context, view graph.ReadOnlyScope) ([]message.Message, error) {
			lastMsg := graph.LastMessage(view)
			input := ""
			if lastMsg != nil {
				input = lastMsg.String()
			}
			fmt.Printf("  [parent] Mapping input to validation subgraph: %s\n", input)
			// Pass the last message to the subgraph
			if lastMsg != nil {
				return []message.Message{lastMsg}, nil
			}
			return nil, nil
		},
		// Output mapper: subgraph output message -> parent state updates
		func(ctx context.Context, output message.Message) (graph.Updates, error) {
			result := ""
			if output != nil {
				result = output.String()
			}
			fmt.Printf("  [parent] Got validation result: %s\n", result)
			return graph.Updates{
				// Store the validated result in messages
				graph.MessagesKeyName: []message.Message{output},
				stepsKey.Name():          []string{"Validation completed"},
			}, nil
		},
	), "run_transform")

	// Use graph.Subgraph() to embed the transformation subgraph
	g.Node("run_transform", graph.Subgraph(
		transformSubgraph,
		// Input mapper: pass the last message to transform
		func(ctx context.Context, view graph.ReadOnlyScope) ([]message.Message, error) {
			lastMsg := graph.LastMessage(view)
			input := ""
			if lastMsg != nil {
				input = lastMsg.String()
			}
			fmt.Printf("  [parent] Mapping input to transform subgraph: %s\n", input)
			if lastMsg != nil {
				return []message.Message{lastMsg}, nil
			}
			return nil, nil
		},
		// Output mapper: get the transformed result
		func(ctx context.Context, output message.Message) (graph.Updates, error) {
			result := ""
			if output != nil {
				result = output.String()
			}
			fmt.Printf("  [parent] Got transform result: %s\n", result)
			return graph.Updates{
				graph.MessagesKeyName: []message.Message{output},
				stepsKey.Name():          []string{"Transform completed"},
			}, nil
		},
	), "finalize")

	// Finalize and show results
	g.Node("finalize", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		lastMsg := graph.LastMessage(scope)
		result := ""
		if lastMsg != nil {
			result = lastMsg.String()
		}
		steps := graph.GetList(scope, stepsKey)

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
	fmt.Println("    • Input/output mappers bridge parent ↔ child messages")
	fmt.Println("    • Subgraphs can be reused across multiple nodes")
}
