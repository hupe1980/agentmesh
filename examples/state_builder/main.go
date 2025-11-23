// Package main demonstrates StateBuilder for simplified state initialization.
package main

import (
	"context"
	"fmt"
	"log"

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

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
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, phaseKey)
	graphstate.RegisterKey(mgr, attemptsKey)
	graphstate.RegisterKey(mgr, validatedKey)
	graphstate.RegisterKey(mgr, actionLogKey.Key)
	graphstate.RegisterKey(mgr, taskResultsKey)

	// Create a simple workflow
	gph, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	gph.AddNode(&graph.BaseCommandNode{
		NodeName:        "init",
		DeclaredTargets: graph.NewTargetSet("process"),
		Fn: func(ctx context.Context, view *graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("[init] Initializing...")
			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, phaseKey, "processing")
			graphstate.SetUpdate(builder, attemptsKey, 1)
			graphstate.AppendUpdate(builder, actionLogKey, "Initialized")
			updates, _ := builder.Build()
			return graph.Goto("process", updates), nil
		},
	})

	gph.AddNode(&graph.BaseCommandNode{
		NodeName:        "process",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view *graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("[process] Processing...")

			// Read current attempts count
			currentAttempts := graphstate.GetFromView(view, attemptsKey)

			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, attemptsKey, currentAttempts+1)
			graphstate.SetUpdate(builder, validatedKey, true)
			graphstate.AppendUpdate(builder, actionLogKey, "Processed")
			graphstate.SetUpdate(builder, taskResultsKey, map[string]any{"process": "success"})
			updates, _ := builder.Build()
			return graph.End(updates), nil
		},
	})

	if err := gph.SetEntryPoint("init"); err != nil {
		panic(err)
	}

	compiled, _ := graph.Compile(gph, graph.NewMessagePregelExecutor())
	ctx := context.Background()
	graph.Last(compiled.Run(ctx, nil))

	// Read final state using typed keys
	view, err := mgr.CreateReadView(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== Final State ===")
	fmt.Printf("Phase: %v\n", graphstate.GetFromView(view, phaseKey))
	fmt.Printf("Attempts: %v\n", graphstate.GetFromView(view, attemptsKey))
	fmt.Printf("Validated: %v\n", graphstate.GetFromView(view, validatedKey))
}
