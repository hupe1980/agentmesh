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

	pipeline.AddStaticNode("init", graph.NewTargetSet("validation"), func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
		data := map[string]any{
			"user_id": "12345",
			"email":   "user@example.com",
			"score":   75,
		}
		builder := graphstate.NewUpdateBuilder()
		graphstate.SetUpdate(builder, inputDataKey, data)
		return builder.Build()
	})

	pipeline.AddStaticNode("validation", graph.NewTargetSet("enrichment"), func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
		data := graphstate.GetFromView(view, inputDataKey)
		builder := graphstate.NewUpdateBuilder()

		required := []string{"user_id", "email", "score"}
		for _, field := range required {
			if _, ok := data[field]; !ok {
				graphstate.SetUpdate(builder, validKey, false)
				return builder.Build()
			}
		}
		score, ok := data["score"].(int)
		if !ok || score < 0 || score > 100 {
			graphstate.SetUpdate(builder, validKey, false)
			return builder.Build()
		}
		graphstate.SetUpdate(builder, validKey, true)
		return builder.Build()
	})

	pipeline.AddStaticNode("enrichment", graph.NewTargetSet("analysis"), func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
		data := graphstate.GetFromView(view, inputDataKey)
		valid := graphstate.GetFromView(view, validKey)
		if !valid {
			return nil, nil
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
		builder := graphstate.NewUpdateBuilder()
		graphstate.SetUpdate(builder, enrichedDataKey, enriched)
		return builder.Build()
	})

	pipeline.AddStaticNode("analysis", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
		enrichedData := graphstate.GetFromView(view, enrichedDataKey)
		analysis := map[string]any{
			"processed":   true,
			"score_grade": enrichedData["grade"],
			"total_items": len(enrichedData),
		}
		builder := graphstate.NewUpdateBuilder()
		graphstate.SetUpdate(builder, analysisKey, analysis)
		return builder.Build()
	})

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
