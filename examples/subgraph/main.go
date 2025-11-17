// Package main demonstrates a multi-stage data processing pipeline.

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

func main() {
	ctx := context.Background()

	dataKey := graphstate.NewKey("data", map[string]any{})
	validKey := graphstate.NewKey("valid", false)
	enrichedDataKey := graphstate.NewKey("enriched_data", map[string]any{})
	analysisKey := graphstate.NewKey("analysis", map[string]any{})

	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, graphstate.MessagesKey.Key)
	graphstate.RegisterKey(mgr, dataKey)
	graphstate.RegisterKey(mgr, validKey)
	graphstate.RegisterKey(mgr, enrichedDataKey)
	graphstate.RegisterKey(mgr, analysisKey)

	pipeline, err := exec.NewBuilder(graph.WithManager(mgr))
	if err != nil {
		log.Fatalf("Failed to create pipeline builder: %v", err)
	}

	pipeline.Node("init", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
		data := map[string]any{
			"user_id": "12345",
			"email":   "user@example.com",
			"score":   75,
		}
		return &graph.NodeResult{
			Updates: graphstate.Updates{dataKey.Name(): data},
		}, nil
	})

	pipeline.Node("validation", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
		data := graphstate.GetFromView(view, dataKey)
		required := []string{"user_id", "email", "score"}
		for _, field := range required {
			if _, ok := data[field]; !ok {
				return &graph.NodeResult{
					Updates: graphstate.Updates{validKey.Name(): false},
				}, nil
			}
		}
		score, ok := data["score"].(int)
		if !ok || score < 0 || score > 100 {
			return &graph.NodeResult{
				Updates: graphstate.Updates{validKey.Name(): false},
			}, nil
		}
		return &graph.NodeResult{
			Updates: graphstate.Updates{validKey.Name(): true},
		}, nil
	})

	pipeline.Node("enrichment", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
		data := graphstate.GetFromView(view, dataKey)
		valid := graphstate.GetFromView(view, validKey)
		if !valid {
			return &graph.NodeResult{}, nil
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
		return &graph.NodeResult{
			Updates: graphstate.Updates{enrichedDataKey.Name(): enriched},
		}, nil
	})

	pipeline.Node("analysis", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
		enrichedData := graphstate.GetFromView(view, enrichedDataKey)
		analysis := map[string]any{
			"processed":   true,
			"score_grade": enrichedData["grade"],
			"total_items": len(enrichedData),
		}
		return &graph.NodeResult{
			Updates: graphstate.Updates{analysisKey.Name(): analysis},
		}, nil
	})

	pipeline.AddEdge(graph.StartNode, "init")
	pipeline.AddEdge("init", "validation")
	pipeline.AddEdge("validation", "enrichment")
	pipeline.AddEdge("enrichment", "analysis")
	pipeline.AddEdge("analysis", graph.EndNode)

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
	view, err := mgr.CreateReadView(ctx)
	if err != nil {
		log.Fatalf("Failed to create read view: %v", err)
	}
	fmt.Printf("Valid: %v\n", graphstate.GetFromView(view, validKey))
	fmt.Printf("Enriched Data: %+v\n", graphstate.GetFromView(view, enrichedDataKey))
	fmt.Printf("Analysis: %+v\n", graphstate.GetFromView(view, analysisKey))
}
