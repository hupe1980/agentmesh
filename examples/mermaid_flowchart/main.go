// Package main demonstrates generating Mermaid flowcharts from graphs.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
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
	g := graph.New()

	g.Node("preprocess", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("process")
	}, "process")

	g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("postprocess")
	}, "postprocess")

	g.Node("postprocess", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)

	g.Start("preprocess")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(compiled.MermaidFlowchart("LR"))
}

func conditionalWorkflow() {
	categoryKey := graph.NewKey[string]("category")

	g := graph.New(categoryKey)

	g.Node("analyze", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		category := "simple"
		if category == "simple" {
			return graph.Set(categoryKey, category).To("simple_path")
		}
		return graph.Set(categoryKey, category).To("complex_path")
	}, "simple_path", "complex_path")

	g.Node("simple_path", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("finalize")
	}, "finalize")

	g.Node("complex_path", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("finalize")
	}, "finalize")

	g.Node("finalize", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)

	g.Start("analyze")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(compiled.MermaidFlowchart("TD"))
}

func parallelWorkflow() {
	g := graph.New()

	g.Node("split", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Fan out to all workers in parallel
		return graph.Cmd().To("worker_1", "worker_2", "worker_3")
	}, "worker_1", "worker_2", "worker_3")

	g.Node("worker_1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("merge")
	}, "merge")

	g.Node("worker_2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("merge")
	}, "merge")

	g.Node("worker_3", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("merge")
	}, "merge")

	g.Node("merge", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)

	g.Start("split")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(compiled.MermaidFlowchart("TD"))
}

func complexWorkflow() {
	validKey := graph.NewKey[bool]("valid")
	priorityKey := graph.NewKey[string]("priority")

	g := graph.New(validKey, priorityKey)

	g.Node("input_validation", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		priority := "high"
		if priority == "high" {
			return graph.Set(validKey, true).
				With(graph.SetValue(priorityKey, priority)).
				To("high_priority")
		}
		return graph.Set(validKey, true).
			With(graph.SetValue(priorityKey, priority)).
			To("normal_priority")
	}, "high_priority", "normal_priority")

	g.Node("high_priority", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("transform")
	}, "transform")

	g.Node("normal_priority", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("transform")
	}, "transform")

	g.Node("transform", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("enrich")
	}, "enrich")

	g.Node("enrich", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("aggregate")
	}, "aggregate")

	g.Node("aggregate", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().To("output")
	}, "output")

	g.Node("output", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)

	g.Start("input_validation")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(compiled.MermaidFlowchart("LR"))

	fmt.Println("\n\n📊 You can copy the above Mermaid code and paste it into:")
	fmt.Println("   - https://mermaid.live/")
	fmt.Println("   - GitHub markdown (surrounded by ```mermaid ... ```)")
	fmt.Println("   - VS Code with Mermaid extension")
}
