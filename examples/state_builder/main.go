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
	// Use make() to ensure consistent map type for atomic.Value
	defaultTaskResults := make(map[string]any)
	taskResultsKey := graphstate.NewKey("task_results", defaultTaskResults)

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

	gph.AddNode(&graph.BaseNode{
		NodeName:        "init",
		DeclaredTargets: []string{"process"},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("[init] Initializing...")
			updates := graphstate.Updates{}
			updates[phaseKey.Name()] = "processing"
			updates[attemptsKey.Name()] = 1
			updates[actionLogKey.Name()] = []string{"Initialized"}
			return []string{"process"}, updates, nil
		},
	})

	gph.AddNode(&graph.BaseNode{
		NodeName:        "process",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("[process] Processing...")

			// Read current attempts count
			currentAttempts := graphstate.GetFromView(view, attemptsKey)

			updates := graphstate.Updates{}
			updates[attemptsKey.Name()] = currentAttempts + 1
			updates[validatedKey.Name()] = true
			updates[actionLogKey.Name()] = []string{"Processed"}
			updates[taskResultsKey.Name()] = map[string]any{"process": "success"}
			return []string{graph.EndNode}, updates, nil
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
