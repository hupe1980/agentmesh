// Package main demonstrates StateBuilder for simplified state initialization.
package main

import (
	"context"
	"fmt"

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

func main() {
	fmt.Println("=== StateBuilder Example ===")
	fmt.Println()

	// Build state with common patterns using fluent API
	state, err := graph.NewStateBuilder().
		WithMessages(50).
		WithLastValue("phase", "initialization").
		WithCounter("attempts").
		WithFlag("validated").
		WithList("action_log").
		WithMap("task_results").
		Build()
	if err != nil {
		panic(err)
	}

	// Create a simple workflow
	gph, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}

	gph.AddNode(&graph.Node{
		Name: "init",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("[init] Initializing...")
			return &graph.NodeResult{
				Updates: map[string]any{
					"phase":      "processing",
					"attempts":   1,
					"action_log": []string{"Initialized"},
				},
			}, nil
		},
	})

	gph.AddNode(&graph.Node{
		Name: "process",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("[process] Processing...")
			return &graph.NodeResult{
				Updates: map[string]any{
					"attempts":     1,
					"validated":    true,
					"action_log":   []string{"Processed"},
					"task_results": map[string]any{"process": "success"},
				},
			}, nil
		},
	})

	gph.AddEdge(graph.StartNode, "init")
	gph.AddEdge("init", "process")
	gph.AddEdge("process", graph.EndNode)

	compiled, _ := exec.CompileGraph(gph)
	graph.Last(compiled.Run(context.Background(), nil))

	fmt.Println("\n=== Final State ===")
	fmt.Printf("Phase: %v\n", state.Get("phase"))
	fmt.Printf("Attempts: %v\n", state.Get("attempts"))
	fmt.Printf("Validated: %v\n", state.Get("validated"))
}
