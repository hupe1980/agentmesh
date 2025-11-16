// Package main demonstrates subgraph.

package main

import (
	"context"
	"fmt"
	"log"
	"maps"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

// Example: Multi-stage data processing pipeline using subgraphs
// Pipeline: Validation -> Enrichment -> Analysis -> Report

func main() {
	ctx := context.Background()

	// Create validation subgraph
	validationSub := createValidationSubgraph()
	compiledValidationImpl, err := validationSub.Compile()
	if err != nil {
		log.Fatalf("Failed to compile validation subgraph: %v", err)
	}
	compiledValidation := compiledValidationImpl.(*exec.RunnableGraph)

	// Create enrichment subgraph
	enrichmentSub := createEnrichmentSubgraph()
	compiledEnrichmentImpl, err := enrichmentSub.Compile()
	if err != nil {
		log.Fatalf("Failed to compile enrichment subgraph: %v", err)
	}
	compiledEnrichment := compiledEnrichmentImpl.(*exec.RunnableGraph)

	// Create analysis subgraph with state mapping
	analysisSub := createAnalysisSubgraph()
	compiledAnalysisImpl, err := analysisSub.Compile()
	if err != nil {
		log.Fatalf("Failed to compile analysis subgraph: %v", err)
	}
	compiledAnalysis := compiledAnalysisImpl.(*exec.RunnableGraph)

	// In Phase 2, we build a main pipeline that calls subgraphs as runnables
	pipeline, err := exec.NewBuilder()
	if err != nil {
		log.Fatalf("Failed to create pipeline builder: %v", err)
	}

	// Wrap subgraphs as nodes that execute the compiled runnables
	pipeline.Node("validation", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		// Run validation subgraph
		_, err := graph.Last(compiledValidation.Run(ctx, nil))
		if err != nil {
			return nil, err
		}
		// Copy validation results to parent state
		valid := compiledValidation.State().Get("valid")
		s.Set("valid", valid)
		return &graph.NodeResult{
			Updates: map[string]any{"valid": valid},
		}, nil
	})

	pipeline.Node("enrichment", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		// Run enrichment subgraph
		_, err := graph.Last(compiledEnrichment.Run(ctx, nil))
		if err != nil {
			return nil, err
		}
		// Copy enriched data to parent state
		enrichedData := compiledEnrichment.State().Get("enriched_data")
		return &graph.NodeResult{
			Updates: map[string]any{"enriched_data": enrichedData},
		}, nil
	})

	pipeline.Node("analysis", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		// Run analysis subgraph
		_, err := graph.Last(compiledAnalysis.Run(ctx, nil))
		if err != nil {
			return nil, err
		}
		// Copy analysis results to parent state
		analysis := compiledAnalysis.State().Get("analysis")
		return &graph.NodeResult{
			Updates: map[string]any{"analysis": analysis},
		}, nil
	})

	pipeline.AddEdge(graph.StartNode, "validation")
	pipeline.AddEdge("validation", "enrichment")
	pipeline.AddEdge("enrichment", "analysis")
	pipeline.AddEdge("analysis", graph.EndNode)

	compiled, err := pipeline.Compile()
	if err != nil {
		log.Fatalf("Failed to compile pipeline: %v", err)
	}
	compiledPipeline := compiled.(*exec.RunnableGraph)

	// Execute pipeline with sample data
	initialState := map[string]any{
		"data": map[string]any{
			"user_id": "12345",
			"email":   "user@example.com",
			"score":   75,
		},
	}

	// Set initial state in all subgraphs
	if err := compiledValidation.ApplyState(initialState); err != nil {
		log.Fatalf("Failed to apply state to validation: %v", err)
	}
	if err := compiledEnrichment.ApplyState(initialState); err != nil {
		log.Fatalf("Failed to apply state to enrichment: %v", err)
	}
	if err := compiledAnalysis.ApplyState(initialState); err != nil {
		log.Fatalf("Failed to apply state to analysis: %v", err)
	}

	_, err = graph.Last(compiledPipeline.Run(ctx, nil))
	if err != nil {
		log.Fatalf("Pipeline execution failed: %v", err)
	}

	// Print results
	fmt.Println("\n=== Pipeline Results ===")
	fmt.Printf("Valid: %v\n", compiledPipeline.State().Get("valid"))
	fmt.Printf("Enriched Data: %+v\n", compiledPipeline.State().Get("enriched_data"))
	fmt.Printf("Analysis: %+v\n", compiledPipeline.State().Get("analysis"))
	fmt.Printf("Report: %s\n", compiledPipeline.State().Get("report"))
}

// createValidationSubgraph validates input data
func createValidationSubgraph() *graph.Graph {
	state, err := graphstate.NewStateManager(0)
	if err != nil {
		panic(err)
	}
	g, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}

	g.AddNode(&graph.Node{
		Name: "validate_structure",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			data, ok := s.Get("data").(map[string]any)
			if !ok {
				return &graph.NodeResult{
					Updates: map[string]any{
						"valid":            false,
						"validation_error": "missing data",
					},
				}, nil
			}

			// Check required fields
			required := []string{"user_id", "email", "score"}
			for _, field := range required {
				if _, ok := data[field]; !ok {
					return &graph.NodeResult{
						Updates: map[string]any{
							"valid":            false,
							"validation_error": fmt.Sprintf("missing field: %s", field),
						},
					}, nil
				}
			}

			return &graph.NodeResult{
				Updates: map[string]any{
					"valid":            true,
					"validation_error": "",
				},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "validate_values",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			if !s.Get("valid").(bool) {
				return &graph.NodeResult{}, nil // Skip if already invalid
			}

			data := s.Get("data").(map[string]any)
			score, ok := data["score"].(int)
			if !ok || score < 0 || score > 100 {
				return &graph.NodeResult{
					Updates: map[string]any{
						"valid":            false,
						"validation_error": "score must be between 0 and 100",
					},
				}, nil
			}

			return &graph.NodeResult{
				Updates: map[string]any{"valid": true},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "validate_structure")
	g.AddEdge("validate_structure", "validate_values")

	return g
}

// createEnrichmentSubgraph adds computed fields to data
func createEnrichmentSubgraph() *graph.Graph {
	state, err := graphstate.NewStateManager(0)
	if err != nil {
		panic(err)
	}
	g, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}

	g.AddNode(&graph.Node{
		Name: "enrich",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			data, ok := s.Get("data").(map[string]any)
			if !ok {
				return &graph.NodeResult{}, nil
			}

			// Create enriched copy with additional fields
			enriched := make(map[string]any)
			maps.Copy(enriched, data)

			// Add computed fields
			score := data["score"].(int)
			if score >= 80 {
				enriched["grade"] = "A"
			} else if score >= 60 {
				enriched["grade"] = "B"
			} else {
				enriched["grade"] = "C"
			}

			enriched["status"] = "active"
			enriched["enriched_at"] = "2024-01-15T10:00:00Z"

			return &graph.NodeResult{
				Updates: map[string]any{
					"enriched_data": enriched,
				},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "enrich")

	return g
}

// createAnalysisSubgraph performs analysis on enriched data
func createAnalysisSubgraph() *graph.Graph {
	state, err := graphstate.NewStateManager(0)
	if err != nil {
		panic(err)
	}
	g, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}

	g.AddNode(&graph.Node{
		Name: "analyze",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			// This subgraph expects "input" key (will be mapped)
			input, ok := s.Get("input").(map[string]any)
			if !ok {
				return &graph.NodeResult{
					Updates: map[string]any{
						"output": map[string]any{"error": "no input"},
					},
				}, nil
			}

			score := input["score"].(int)
			grade := input["grade"].(string)

			analysis := map[string]any{
				"score":          score,
				"grade":          grade,
				"performance":    getPerformanceLevel(score),
				"recommendation": getRecommendation(score),
			}

			return &graph.NodeResult{
				Updates: map[string]any{
					"output": analysis,
				},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "analyze")

	return g
}

func getPerformanceLevel(score int) string {
	if score >= 90 {
		return "Excellent"
	} else if score >= 70 {
		return "Good"
	} else if score >= 50 {
		return "Fair"
	}
	return "Needs Improvement"
}

func getRecommendation(score int) string {
	if score >= 80 {
		return "Maintain current performance"
	} else if score >= 60 {
		return "Focus on weak areas for improvement"
	}
	return "Consider additional training and support"
}
