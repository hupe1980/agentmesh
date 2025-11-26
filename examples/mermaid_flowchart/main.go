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
		AddNodeFunc("preprocess", []string{"process"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"process"}, nil, nil
		}).
		AddNodeFunc("process", []string{"postprocess"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"postprocess"}, nil, nil
		}).
		AddNodeFunc("postprocess", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
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
		AddNodeFunc("analyze", []string{"simple_path", "complex_path"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			updates := state.Updates{}
			updates[categoryKey.Name()] = "simple"

			category := "simple" // Just set it above
			if category == "simple" {
				return []string{"simple_path"}, updates, nil
			}
			return []string{"complex_path"}, updates, nil
		}).
		AddNodeFunc("simple_path", []string{"finalize"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"finalize"}, nil, nil
		}).
		AddNodeFunc("complex_path", []string{"finalize"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"finalize"}, nil, nil
		}).
		AddNodeFunc("finalize", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
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
		AddNodeFunc("split", []string{"worker_1", "worker_2", "worker_3"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"worker_1", "worker_2", "worker_3"}, nil, nil
		}).
		AddNodeFunc("worker_1", []string{"merge"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"merge"}, nil, nil
		}).
		AddNodeFunc("worker_2", []string{"merge"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"merge"}, nil, nil
		}).
		AddNodeFunc("worker_3", []string{"merge"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"merge"}, nil, nil
		}).
		AddNodeFunc("merge", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
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
		AddNodeFunc("input_validation", []string{"high_priority", "normal_priority"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			updates := state.Updates{}
			updates[validKey.Name()] = true
			updates[priorityKey.Name()] = "high"

			priority := "high" // Just set it above
			if priority == "high" {
				return []string{"high_priority"}, updates, nil
			}
			return []string{"normal_priority"}, updates, nil
		}).
		AddNodeFunc("high_priority", []string{"transform"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"transform"}, nil, nil
		}).
		AddNodeFunc("normal_priority", []string{"transform"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"transform"}, nil, nil
		}).
		AddNodeFunc("transform", []string{"enrich"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"enrich"}, nil, nil
		}).
		AddNodeFunc("enrich", []string{"aggregate"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"aggregate"}, nil, nil
		}).
		AddNodeFunc("aggregate", []string{"output"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{"output"}, nil, nil
		}).
		AddNodeFunc("output", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
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
