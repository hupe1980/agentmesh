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
	var defaultMap map[string]any
	taskResultsKey := graphstate.NewKey("task_results", defaultMap)

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
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("[init] Initializing...")
			builder := graph.NewCommand()
			graph.CommandSet(builder, phaseKey, "processing")
			graph.CommandSet(builder, attemptsKey, 1)
			graph.CommandAppend(builder, actionLogKey, "Initialized")
			return builder.Goto("process")
		},
	})

	gph.AddNode(&graph.BaseCommandNode{
		NodeName:        "process",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("[process] Processing...")

			// Read current attempts count
			currentAttempts := graphstate.GetFromView(view, attemptsKey)

			builder := graph.NewCommand()
			graph.CommandSet(builder, attemptsKey, currentAttempts+1)
			graph.CommandSet(builder, validatedKey, true)
			graph.CommandAppend(builder, actionLogKey, "Processed")
			graph.CommandSet(builder, taskResultsKey, map[string]any{"process": "success"})
			return builder.End()
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
