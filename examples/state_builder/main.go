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

	// Define typed keys for state management
	phaseKey := graphstate.NewKey("phase", "initialization")
	attemptsKey := graphstate.NewKey("attempts", 0)
	validatedKey := graphstate.NewKey("validated", false)
	actionLogKey := graphstate.NewListKey[string]("action_log", 0)
	taskResultsKey := graphstate.NewKey("task_results", map[string]any{})

	// Create state and register keys
	st := graphstate.NewState()
	graphstate.Register(st, phaseKey)
	graphstate.Register(st, attemptsKey)
	graphstate.Register(st, validatedKey)
	graphstate.Register(st, actionLogKey.Key)
	graphstate.Register(st, taskResultsKey)

	// Create a simple workflow
	gph, err := graph.NewGraph(st)
	if err != nil {
		panic(err)
	}

	gph.AddNode(&graph.Node{
		Name: "init",
		RunFunc: func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
			fmt.Println("[init] Initializing...")
			return &graph.NodeResult{
				Updates: graphstate.Updates{
					phaseKey.Name():     "processing",
					attemptsKey.Name():  1,
					actionLogKey.Name(): []string{"Initialized"},
				},
			}, nil
		},
	})

	gph.AddNode(&graph.Node{
		Name: "process",
		RunFunc: func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
			fmt.Println("[process] Processing...")

			// Read current attempts count
			currentAttempts := graphstate.GetFromView(view, attemptsKey)

			return &graph.NodeResult{
				Updates: graphstate.Updates{
					attemptsKey.Name():    currentAttempts + 1,
					validatedKey.Name():   true,
					actionLogKey.Name():   []string{"Processed"},
					taskResultsKey.Name(): map[string]any{"process": "success"},
				},
			}, nil
		},
	})

	gph.AddEdge(graph.StartNode, "init")
	gph.AddEdge("init", "process")
	gph.AddEdge("process", graph.EndNode)

	compiled, _ := exec.CompileGraph(gph)
	graph.Last(compiled.Run(context.Background(), nil))

	// Read final state using typed keys
	snap := st.Snapshot()
	view := graphstate.NewReadView(snap)

	fmt.Println("\n=== Final State ===")
	fmt.Printf("Phase: %v\n", graphstate.GetFromView(view, phaseKey))
	fmt.Printf("Attempts: %v\n", graphstate.GetFromView(view, attemptsKey))
	fmt.Printf("Validated: %v\n", graphstate.GetFromView(view, validatedKey))
}
