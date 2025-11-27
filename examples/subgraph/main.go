// Package main demonstrates composing graphs with SubgraphNode for type-safe subgraph composition.
// This example shows how to:
//   - Build isolated subgraphs with their own state management
//   - Use type-safe input/output mappers to exchange data between parent and subgraph
//   - Compose subgraphs into a parent pipeline
//   - Organize reusable subgraphs as functions

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Input/Output types for subgraphs
type ValidationInput struct {
	Data map[string]interface{}
}

type ValidationOutput struct {
	Valid  bool
	Errors []string
}

type EnrichmentInput struct {
	Data map[string]interface{}
}

type EnrichmentOutput struct {
	Enriched map[string]interface{}
}

// buildValidationSubgraph creates a reusable validation subgraph
func buildValidationSubgraph() (*graph.Compiled[ValidationInput, ValidationOutput], error) {
	// Create isolated state manager for subgraph
	manager := state.NewManager()

	// Subgraph-internal keys
	inputKey := state.NewKey[map[string]interface{}]("input_data", nil)
	validKey := state.NewKey[bool]("valid", false)
	errorsKey := state.NewListKey[string]("errors", 0)

	state.RegisterKey(manager, inputKey)
	state.RegisterKey(manager, validKey)
	state.RegisterListKey(manager, errorsKey)

	// Create graph
	subgraph, err := graph.NewGraph(manager)
	if err != nil {
		return nil, err
	}

	// Add validation logic
	subgraph.AddNode(&graph.BaseNode{
		NodeName:        "check_required",
		DeclaredTargets: []string{"check_values"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			data := state.GetFromView(view, inputKey)

			fmt.Println("  [Validation] Checking required fields...")

			var errors []string
			required := []string{"user_id", "email", "score"}
			for _, field := range required {
				if _, ok := data[field]; !ok {
					errors = append(errors, fmt.Sprintf("missing field: %s", field))
				}
			}

			return []string{"check_values"}, state.Updates{
				errorsKey.Name(): errors,
			}, nil
		},
	})

	subgraph.AddNode(&graph.BaseNode{
		NodeName:        "check_values",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			data := state.GetFromView(view, inputKey)
			currentErrors := state.GetFromView(view, errorsKey.Key)

			fmt.Println("  [Validation] Checking value constraints...")

			if score, ok := data["score"].(float64); ok {
				if score < 0 || score > 100 {
					currentErrors = append(currentErrors, "score must be between 0 and 100")
				}
			}

			valid := len(currentErrors) == 0
			fmt.Printf("  [Validation] Result: valid=%v, errors=%d\n", valid, len(currentErrors))

			return []string{graph.EndNode}, state.Updates{
				validKey.Name():  valid,
				errorsKey.Name(): currentErrors,
			}, nil
		},
	})

	subgraph.SetEntryPoint("check_required")

	// Create executor with type-safe I/O
	executor := graph.NewPregelExecutor(
		func(input ValidationInput) state.Updates {
			return state.Updates{
				inputKey.Name(): input.Data,
			}
		},
		errorsKey.Name(),
		func(val any) ValidationOutput {
			manager := subgraph.Manager()
			view, _ := manager.CreateReadView(context.Background())

			return ValidationOutput{
				Valid:  state.GetFromView(view, validKey),
				Errors: state.GetFromView(view, errorsKey.Key),
			}
		},
	)

	return graph.Compile(subgraph, executor)
}

// buildEnrichmentSubgraph creates a reusable enrichment subgraph
func buildEnrichmentSubgraph() (*graph.Compiled[EnrichmentInput, EnrichmentOutput], error) {
	manager := state.NewManager()

	inputKey := state.NewKey[map[string]interface{}]("input", nil)
	metadataKey := state.NewKey[map[string]interface{}]("metadata", nil)
	resultKey := state.NewKey[map[string]interface{}]("result", nil)

	state.RegisterKey(manager, inputKey)
	state.RegisterKey(manager, metadataKey)
	state.RegisterKey(manager, resultKey)

	subgraph, err := graph.NewGraph(manager)
	if err != nil {
		return nil, err
	}

	subgraph.AddNode(&graph.BaseNode{
		NodeName:        "add_metadata",
		DeclaredTargets: []string{"calculate_grade"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			fmt.Println("  [Enrichment] Adding metadata...")

			metadata := map[string]interface{}{
				"timestamp": "2024-01-01T00:00:00Z",
				"enriched":  true,
				"version":   "2.0",
			}

			return []string{"calculate_grade"}, state.Updates{
				metadataKey.Name(): metadata,
			}, nil
		},
	})

	subgraph.AddNode(&graph.BaseNode{
		NodeName:        "calculate_grade",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			data := state.GetFromView(view, inputKey)
			metadata := state.GetFromView(view, metadataKey)

			fmt.Println("  [Enrichment] Calculating grade...")

			result := make(map[string]interface{})
			for k, v := range data {
				result[k] = v
			}
			for k, v := range metadata {
				result[k] = v
			}

			// Add grade based on score
			if score, ok := data["score"].(float64); ok {
				var grade string
				if score >= 90 {
					grade = "A"
				} else if score >= 80 {
					grade = "B"
				} else if score >= 70 {
					grade = "C"
				} else {
					grade = "D"
				}
				result["grade"] = grade
			}

			fmt.Printf("  [Enrichment] Result: %v\n", result)

			return []string{graph.EndNode}, state.Updates{
				resultKey.Name(): result,
			}, nil
		},
	})

	subgraph.SetEntryPoint("add_metadata")

	executor := graph.NewPregelExecutor(
		func(input EnrichmentInput) state.Updates {
			return state.Updates{
				inputKey.Name(): input.Data,
			}
		},
		resultKey.Name(),
		func(val any) EnrichmentOutput {
			if enriched, ok := val.(map[string]interface{}); ok {
				return EnrichmentOutput{Enriched: enriched}
			}
			return EnrichmentOutput{Enriched: make(map[string]interface{})}
		},
	)

	return graph.Compile(subgraph, executor)
}

func main() {
	fmt.Println("=== SubgraphNode Composition Example ===")
	fmt.Println()

	ctx := context.Background()

	// Build reusable subgraphs
	fmt.Println("Building subgraphs...")
	validationCompiled, err := buildValidationSubgraph()
	if err != nil {
		log.Fatalf("Failed to build validation subgraph: %v", err)
	}

	enrichmentCompiled, err := buildEnrichmentSubgraph()
	if err != nil {
		log.Fatalf("Failed to build enrichment subgraph: %v", err)
	}
	fmt.Println()

	// Create parent graph state keys
	parentManager := state.NewManager()
	dataKey := state.NewKey[map[string]interface{}]("data", nil)
	validKey := state.NewKey[bool]("valid", false)
	errorsKey := state.NewListKey[string]("errors", 0)
	enrichedKey := state.NewKey[map[string]interface{}]("enriched", nil)
	statusKey := state.NewKey[string]("status", "")

	state.RegisterKey(parentManager, dataKey)
	state.RegisterKey(parentManager, validKey)
	state.RegisterListKey(parentManager, errorsKey)
	state.RegisterKey(parentManager, enrichedKey)
	state.RegisterKey(parentManager, statusKey)

	// Create SubgraphNodes with type-safe mappers
	validationNode := graph.NewSubgraphNode(
		"validation",
		validationCompiled,
		// Input mapper: parent state → ValidationInput
		func(ctx context.Context, view state.ReadView) (ValidationInput, error) {
			return ValidationInput{
				Data: state.GetFromView(view, dataKey),
			}, nil
		},
		// Output mapper: ValidationOutput → parent state updates
		func(ctx context.Context, output ValidationOutput) (state.Updates, error) {
			return state.Updates{
				validKey.Name():  output.Valid,
				errorsKey.Name(): output.Errors,
			}, nil
		},
		[]string{"enrichment", graph.EndNode},
		graph.WithSubgraphVersion("1.0.0"),
		graph.WithSubgraphMetadata("description", "Data validation subgraph"),
	)

	enrichmentNode := graph.NewSubgraphNode(
		"enrichment",
		enrichmentCompiled,
		func(ctx context.Context, view state.ReadView) (EnrichmentInput, error) {
			return EnrichmentInput{
				Data: state.GetFromView(view, dataKey),
			}, nil
		},
		func(ctx context.Context, output EnrichmentOutput) (state.Updates, error) {
			return state.Updates{
				enrichedKey.Name(): output.Enriched,
			}, nil
		},
		[]string{"output"},
		graph.WithSubgraphVersion("2.0.0"),
		graph.WithSubgraphMetadata("description", "Data enrichment subgraph"),
	)

	// Build parent pipeline with explicit manager
	builder, err := graph.NewBuilder(
		graph.NewMessagePregelExecutor(),
		graph.WithManager[[]message.Message, message.Message](parentManager),
	)
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Input node
	builder.AddNodeFunc("input", []string{"validation"},
		func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			fmt.Println("[Input] Processing initial data")

			data := map[string]interface{}{
				"user_id": "12345",
				"email":   "user@example.com",
				"score":   85.5,
			}

			return []string{"validation"}, state.Updates{
				dataKey.Name():   data,
				statusKey.Name(): "processing",
			}, nil
		},
	)

	// Add subgraph nodes
	builder.AddSubgraphNode(validationNode)
	builder.AddSubgraphNode(enrichmentNode)

	// Output node
	builder.AddNodeFunc("output", []string{graph.EndNode},
		func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			valid := state.GetFromView(view, validKey)
			errors := state.GetFromView(view, errorsKey.Key)
			enriched := state.GetFromView(view, enrichedKey)

			fmt.Println("\n[Output] Final results:")
			fmt.Printf("  Valid: %v\n", valid)
			if len(errors) > 0 {
				fmt.Printf("  Errors: %v\n", errors)
			}
			fmt.Printf("  Enriched data: %v\n", enriched)

			return []string{graph.EndNode}, state.Updates{
				statusKey.Name(): "completed",
			}, nil
		},
	)

	builder.SetEntryPoint("input")

	// Compile parent graph
	fmt.Println("Compiling parent pipeline...")
	compiled, err := builder.Compile()
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}
	fmt.Println()

	// Execute pipeline
	fmt.Println("Executing pipeline with SubgraphNode composition:")
	fmt.Println()

	for _, err := range compiled.Run(ctx, []message.Message{}) {
		if err != nil {
			log.Printf("Error: %v", err)
			break
		}
	}

	fmt.Println("\n=== Pipeline Complete ===")
	fmt.Println("\nKey takeaways:")
	fmt.Println("  • Each subgraph has isolated state (cannot access parent state)")
	fmt.Println("  • Type-safe input/output mappers exchange data")
	fmt.Println("  • Subgraphs are reusable across multiple graphs")
	fmt.Println("  • Version tracking for compatibility")
}
