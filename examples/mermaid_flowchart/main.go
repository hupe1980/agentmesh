// Package main demonstrates generating Mermaid flowcharts from graphs.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	fmt.Println("=== Example 1: Simple Linear Workflow ===")
	simpleWorkflow()

	fmt.Println("\n\n=== Example 2: Conditional Routing ===")
	conditionalWorkflow()

	fmt.Println("\n\n=== Example 3: Parallel Execution ===")
	parallelWorkflow()

	fmt.Println("\n\n=== Example 4: Complex Workflow ===")
	complexWorkflow()
}

func simpleWorkflow() {
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	builder.
		AddStaticNode("preprocess", graph.NewTargetSet("process"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("process", graph.NewTargetSet("postprocess"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("postprocess", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		SetEntryPoint("preprocess")

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatal(err)
	}
	rg := compiled
	fmt.Println(rg.MermaidFlowchart("LR"))
}

func conditionalWorkflow() {
	categoryKey := state.NewKey("category", "")

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	builder.
		AddCommandNode("analyze", graph.NewTargetSet("simple_path", "complex_path"), func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			b := graph.NewUpdate()
			graph.UpdateSet(b, categoryKey, "simple")
			updates, _ := b.Build()

			category := "simple" // Just set it above
			if category == "simple" {
				return graph.Goto("simple_path", updates), nil
			}
			return graph.Goto("complex_path", updates), nil
		}).
		AddStaticNode("simple_path", graph.NewTargetSet("finalize"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("complex_path", graph.NewTargetSet("finalize"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("finalize", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		SetEntryPoint("analyze")

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatal(err)
	}
	rg := compiled
	fmt.Println(rg.MermaidFlowchart("TD"))
}

func parallelWorkflow() {
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	builder.
		AddStaticNode("split", graph.NewTargetSet("worker_1", "worker_2", "worker_3"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("worker_1", graph.NewTargetSet("merge"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("worker_2", graph.NewTargetSet("merge"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("worker_3", graph.NewTargetSet("merge"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("merge", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		SetEntryPoint("split")

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatal(err)
	}
	rg := compiled
	fmt.Println(rg.MermaidFlowchart("TD"))
}

func complexWorkflow() {
	validKey := state.NewKey("valid", false)
	priorityKey := state.NewKey("priority", "")

	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	builder.
		AddCommandNode("input_validation", graph.NewTargetSet("high_priority", "normal_priority"), func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			b := graph.NewUpdate()
			graph.UpdateSet(b, validKey, true)
			graph.UpdateSet(b, priorityKey, "high")
			updates, _ := b.Build()

			priority := "high" // Just set it above
			if priority == "high" {
				return graph.Goto("high_priority", updates), nil
			}
			return graph.Goto("normal_priority", updates), nil
		}).
		AddStaticNode("high_priority", graph.NewTargetSet("transform"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("normal_priority", graph.NewTargetSet("transform"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("transform", graph.NewTargetSet("enrich"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("enrich", graph.NewTargetSet("aggregate"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("aggregate", graph.NewTargetSet("output"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddStaticNode("output", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		SetEntryPoint("input_validation")

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatal(err)
	}
	rg := compiled
	fmt.Println(rg.MermaidFlowchart("LR"))

	fmt.Println("\n\n📊 You can copy the above Mermaid code and paste it into:")
	fmt.Println("   - https://mermaid.live/")
	fmt.Println("   - GitHub markdown (surrounded by ```mermaid ... ```)")
	fmt.Println("   - VS Code with Mermaid extension")
}
