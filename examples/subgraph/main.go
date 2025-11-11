// Package main demonstrates subgraph.

package main

import (
	"context"
	"fmt"
	"log"
	"maps"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Example: Multi-stage data processing pipeline using subgraphs
// Pipeline: Validation -> Enrichment -> Analysis -> Report

func main() {
	ctx := context.Background()

	// Create validation subgraph
	validationSub := createValidationSubgraph()
	compiledValidation, err := validationSub.Compile()
	if err != nil {
		log.Fatalf("Failed to compile validation subgraph: %v", err)
	}

	// Create enrichment subgraph
	enrichmentSub := createEnrichmentSubgraph()
	compiledEnrichment, err := enrichmentSub.Compile()
	if err != nil {
		log.Fatalf("Failed to compile enrichment subgraph: %v", err)
	}

	// Create analysis subgraph with state mapping
	analysisSub := createAnalysisSubgraph()
	compiledAnalysis, err := analysisSub.Compile()
	if err != nil {
		log.Fatalf("Failed to compile analysis subgraph: %v", err)
	}

	// Create main pipeline
	pipeline := createPipeline(compiledValidation, compiledEnrichment, compiledAnalysis)
	compiled, err := pipeline.Compile()
	if err != nil {
		log.Fatalf("Failed to compile pipeline: %v", err)
	}

	// Execute pipeline with sample data
	initialState := map[string]any{
		"data": map[string]any{
			"user_id": "12345",
			"email":   "user@example.com",
			"score":   75,
		},
	}

	compiled.ApplyState(initialState, nil)

	_, err = graph.Last(compiled.Run(ctx, nil))
	if err != nil {
		log.Fatalf("Pipeline execution failed: %v", err)
	}

	// Print results
	fmt.Println("\n=== Pipeline Results ===")
	fmt.Printf("Valid: %v\n", compiled.State().Get("valid"))
	fmt.Printf("Enriched Data: %+v\n", compiled.State().Get("enriched_data"))
	fmt.Printf("Analysis: %+v\n", compiled.State().Get("analysis"))
	fmt.Printf("Report: %s\n", compiled.State().Get("report"))
}

// createValidationSubgraph validates input data
func createValidationSubgraph() *graph.Graph {
	state := graph.NewStateManager(0)
	g := graph.NewGraph(state)

	g.AddNode(&graph.Node{
		Name: "validate_structure",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
	state := graph.NewStateManager(0)
	g := graph.NewGraph(state)

	g.AddNode(&graph.Node{
		Name: "enrich",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
	state := graph.NewStateManager(0)
	g := graph.NewGraph(state)

	g.AddNode(&graph.Node{
		Name: "analyze",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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

// createPipeline assembles subgraphs into a processing pipeline
func createPipeline(validation, enrichment, analysis *graph.Compiled) *graph.Graph {
	state := graph.NewStateManager(0)
	g := graph.NewGraph(state)

	// Add validation subgraph (direct embedding)
	g.AddNode(validation.AsNode("validation"))

	// Add enrichment subgraph (direct embedding)
	g.AddNode(enrichment.AsNode("enrichment"))

	// Add analysis subgraph with state mapping
	analysisNode := analysis.AsNodeWithStateMapping(
		"analysis",
		// mapInput: map enriched_data to analysis input
		func(s graph.StateReader) (map[string]any, []graph.Event) {
			enrichedData, ok := s.Get("enriched_data").(map[string]any)
			if !ok {
				return map[string]any{"input": map[string]any{}}, nil
			}
			return map[string]any{"input": enrichedData}, nil
		},
		// mapOutput: map analysis output to parent analysis key
		func(s graph.StateReader) (map[string]any, []graph.Event) {
			output, ok := s.Get("output").(map[string]any)
			if !ok {
				return map[string]any{"analysis": map[string]any{}}, nil
			}
			return map[string]any{"analysis": output}, nil
		},
	)
	g.AddNode(analysisNode)

	// Add report generation node
	g.AddNode(&graph.Node{
		Name: "generate_report",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			analysis, ok := s.Get("analysis").(map[string]any)
			if !ok {
				return &graph.NodeResult{
					Updates: map[string]any{"report": "No analysis available"},
				}, nil
			}

			report := fmt.Sprintf(
				"User Analysis Report\n"+
					"-------------------\n"+
					"Score: %v\n"+
					"Grade: %v\n"+
					"Performance: %v\n"+
					"Recommendation: %v",
				analysis["score"],
				analysis["grade"],
				analysis["performance"],
				analysis["recommendation"],
			)

			return &graph.NodeResult{
				Updates: map[string]any{"report": report},
			}, nil
		},
	})

	// Build pipeline flow
	g.AddEdge(graph.StartNode, "validation")

	// Conditional: only proceed if valid
	g.AddConditionalEdges("validation", func(_ context.Context, s graph.StateReader) []string {
		valid, ok := s.Get("valid").(bool)
		if !ok || !valid {
			return []string{graph.EndNode}
		}
		return []string{"enrichment"}
	}, []string{"enrichment", graph.EndNode})

	g.AddEdge("enrichment", "analysis")
	g.AddEdge("analysis", "generate_report")

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
