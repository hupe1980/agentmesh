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
		Node("preprocess", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("process", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("postprocess", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddEdge(graph.StartNode, "preprocess").
		AddEdge("preprocess", "process").
		AddEdge("process", "postprocess").
		AddEdge("postprocess", graph.EndNode)

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
		Node("analyze", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return state.Updates{categoryKey.Name(): "simple"}, nil
		}).
		Node("simple_path", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("complex_path", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("finalize", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddEdge(graph.StartNode, "analyze").
		AddConditionalEdges("analyze", func(ctx context.Context, view *state.ReadView) []string {
			category := state.GetFromView(view, categoryKey)
			if category == "simple" {
				return []string{"simple_path"}
			}
			return []string{"complex_path"}
		}, []string{"simple_path", "complex_path"}).
		AddEdge("simple_path", "finalize").
		AddEdge("complex_path", "finalize").
		AddEdge("finalize", graph.EndNode)

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
		Node("split", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("worker_1", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("worker_2", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("worker_3", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("merge", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddEdge(graph.StartNode, "split").
		AddEdge("split", "worker_1").
		AddEdge("split", "worker_2").
		AddEdge("split", "worker_3").
		AddEdge("worker_1", "merge").
		AddEdge("worker_2", "merge").
		AddEdge("worker_3", "merge").
		AddEdge("merge", graph.EndNode)

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
		Node("input_validation", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return state.Updates{validKey.Name(): true, priorityKey.Name(): "high"}, nil
		}).
		Node("high_priority", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("normal_priority", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("transform", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("enrich", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("aggregate", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		Node("output", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		}).
		AddEdge(graph.StartNode, "input_validation").
		AddConditionalEdges("input_validation", func(ctx context.Context, view *state.ReadView) []string {
			priority := state.GetFromView(view, priorityKey)
			if priority == "high" {
				return []string{"high_priority"}
			}
			return []string{"normal_priority"}
		}, []string{"high_priority", "normal_priority"}).
		AddEdge("high_priority", "transform").
		AddEdge("normal_priority", "transform").
		AddEdge("transform", "enrich").
		AddEdge("enrich", "aggregate").
		AddEdge("aggregate", "output").
		AddEdge("output", graph.EndNode)

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
