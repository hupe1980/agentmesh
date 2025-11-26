// Package main demonstrates a multi-stage data processing pipeline with namespace isolation.
// This example shows how to use namespaces to isolate state between pipeline stages.

package main

import (
	"context"
	"fmt"
	"log"
	"maps"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	ctx := context.Background()

	// Create namespaces for each pipeline stage to isolate state
	// This prevents keys from different stages interfering with each other
	validationNS := graphstate.MustNamespace("validation")
	enrichmentNS := graphstate.MustNamespace("enrichment")
	analysisNS := graphstate.MustNamespace("analysis")

	// Define namespaced keys - each stage has its own "data" key
	// validation.data, enrichment.data, analysis.data
	inputDataKey := graphstate.TypedKey[map[string]any](validationNS, "data", map[string]any{})
	validKey := graphstate.TypedKey[bool](validationNS, "valid", false)
	enrichedDataKey := graphstate.TypedKey[map[string]any](enrichmentNS, "data", map[string]any{})
	analysisKey := graphstate.TypedKey[map[string]any](analysisNS, "result", map[string]any{})

	mgr := graphstate.NewManager()
	if err := agent.RegisterMessagesKey(mgr); err != nil {
		log.Fatal(err)
	}
	graphstate.RegisterKey(mgr, inputDataKey)
	graphstate.RegisterKey(mgr, validKey)
	graphstate.RegisterKey(mgr, enrichedDataKey)
	graphstate.RegisterKey(mgr, analysisKey)

	pipeline, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		log.Fatalf("Failed to create pipeline builder: %v", err)
	}

	pipeline.SetEntryPoint("init")

	pipeline.AddStaticNode("init", []string{"validation"}, func(ctx context.Context, view graphstate.ReadView) (graphstate.Updates, error) {
		data := map[string]any{
			"user_id": "12345",
			"email":   "user@example.com",
			"score":   75,
		}
		return graph.NewCommand().
			Set(inputDataKey, data).
			Build()
	})

	// Use NamespacedNode for validation stage
	// This node can only access validation.* keys
	validationNode := graph.NewNamespacedNode(
		"validation",
		validationNS,
		func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			data := graphstate.GetFromView(view, inputDataKey)

			required := []string{"user_id", "email", "score"}
			for _, field := range required {
				if _, ok := data[field]; !ok {
					return graph.NewCommand().Set(validKey, false).To("enrichment")
				}
			}
			score, ok := data["score"].(int)
			if !ok || score < 0 || score > 100 {
				return graph.NewCommand().Set(validKey, false).To("enrichment")
			}
			return graph.NewCommand().Set(validKey, true).To("enrichment")
		},
		[]string{"enrichment"},
		false, // Don't include global state
	)
	pipeline.AddNode(validationNode)

	// Use NamespacedNode for enrichment stage
	// This node can only access enrichment.* keys (and reads validation.valid)
	enrichmentNode := graph.NewNamespacedNode(
		"enrichment",
		enrichmentNS,
		func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			data := graphstate.GetFromView(view, inputDataKey)
			valid := graphstate.GetFromView(view, validKey)
			if !valid {
				return []string{"analysis"}, nil, nil
			}
			enriched := make(map[string]any)
			maps.Copy(enriched, data)
			score := data["score"].(int)
			if score >= 80 {
				enriched["grade"] = "A"
			} else if score >= 60 {
				enriched["grade"] = "B"
			} else {
				enriched["grade"] = "C"
			}
			enriched["status"] = "enriched"
			return graph.NewCommand().
				Set(enrichedDataKey, enriched).
				To("analysis")
		},
		[]string{"analysis"},
		false, // Don't include global state
	)
	pipeline.AddNode(enrichmentNode)

	// Use NamespacedNode for analysis stage
	// This node can only access analysis.* keys
	analysisNode := graph.NewNamespacedNode(
		"analysis",
		analysisNS,
		func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			enrichedData := graphstate.GetFromView(view, enrichedDataKey)
			analysis := map[string]any{
				"processed":   true,
				"score_grade": enrichedData["grade"],
				"total_items": len(enrichedData),
			}
			return graph.NewCommand().
				Set(analysisKey, analysis).
				To(graph.EndNode)
		},
		[]string{graph.EndNode},
		false, // Don't include global state
	)
	pipeline.AddNode(analysisNode)

	compiled, err := pipeline.Compile()
	if err != nil {
		log.Fatalf("Failed to compile pipeline: %v", err)
	}

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatalf("Pipeline execution failed: %v", err)
		}
	}

	fmt.Println("\n=== Pipeline Results ===")
	fmt.Println("Note: Each stage has isolated state in its own namespace:")
	fmt.Println("  - validation.data, validation.valid")
	fmt.Println("  - enrichment.data")
	fmt.Println("  - analysis.result")
	view, err := mgr.CreateReadView(ctx)
	if err != nil {
		log.Fatalf("Failed to create read view: %v", err)
	}
	fmt.Printf("Valid: %v\n", graphstate.GetFromView(view, validKey))
	fmt.Printf("Enriched Data: %+v\n", graphstate.GetFromView(view, enrichedDataKey))
	fmt.Printf("Analysis: %+v\n", graphstate.GetFromView(view, analysisKey))
}
