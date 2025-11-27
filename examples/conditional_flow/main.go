// Package main demonstrates conditional routing and dynamic branching in AgentMesh graphs.
// This example shows how to:
//   - Create conditional edges that route to different nodes based on state
//   - Use TopicChannel for accumulating action history
//   - Build decision trees with branching logic
//   - Execute the same graph with different inputs to show routing variations
//
// Key concepts:
//   - Command-based routing: Dynamic routing via graph.Goto() in node logic
//   - Reader: Read state values to make routing decisions
//   - TopicChannel: Accumulate values without overwriting (like a list)
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/command"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	fmt.Println("This example demonstrates conditional routing with two different paths.")
	fmt.Println()

	// Execute the same graph with different choices to demonstrate branching
	runScenario("path_a")
	fmt.Println()
	runScenario("path_b")
}

func runScenario(choice string) {
	fmt.Printf("=== Conditional Flow Example: %s ===\n", choice)

	// Define typed keys for state management
	choiceKey := graphstate.NewKey("choice", "")
	nextPathKey := graphstate.NewKey("next_path", "")
	actionHistoryKey := graphstate.NewListKey[string]("action_history", 0)

	// Create state and register keys
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, choiceKey)
	graphstate.RegisterKey(mgr, nextPathKey)
	graphstate.RegisterKey(mgr, actionHistoryKey.Key)

	// Set initial values using Command pattern
	initCtx := context.Background()
	if err := graphstate.Set(initCtx, mgr, choiceKey, choice); err != nil {
		panic(err)
	}
	if err := graphstate.Set(initCtx, mgr, nextPathKey, ""); err != nil {
		panic(err)
	}

	// Create the graph and helper function for adding nodes
	gph, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}
	mustAddNode := func(n graph.Node) {
		if err := gph.AddNode(n); err != nil {
			panic(err)
		}
	}

	// Decision node: Reads input and decides which path to take
	mustAddNode(&graph.BaseNode{
		NodeName:        "decide",
		DeclaredTargets: []string{"path_a", "path_b"},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			choiceVal := graphstate.GetFromView(view, choiceKey)
			fmt.Printf("  [decide] Evaluating choice: %s\n", choiceVal)

			// Update state to indicate which path should be taken
			cmd := command.New().
				Set(nextPathKey, choiceVal).
				Set(actionHistoryKey, append(graphstate.GetFromView(view, actionHistoryKey.Key), fmt.Sprintf("Decision: route to %s", choiceVal)))

			// Route to the chosen path
			if choiceVal == "path_a" {
				return cmd.To("path_a")
			}
			return cmd.To("path_b")
		},
	})

	// Path A: Specialized processing for option A
	mustAddNode(&graph.BaseNode{
		NodeName:        "path_a",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("  [path_a] Executing Path A logic...")
			return command.New().
				Set(actionHistoryKey, append(graphstate.GetFromView(view, actionHistoryKey.Key), "Completed: Path A")).
				To(graph.EndNode)
		},
	})

	// Path B: Alternative processing for option B
	mustAddNode(&graph.BaseNode{
		NodeName:        "path_b",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("  [path_b] Executing Path B logic...")
			return command.New().
				Set(actionHistoryKey, append(graphstate.GetFromView(view, actionHistoryKey.Key), "Completed: Path B")).
				To(graph.EndNode)
		},
	})

	// Set entry point - Command pattern handles all routing internally
	if err := gph.SetEntryPoint("decide"); err != nil {
		panic(err)
	}

	// Compile the graph into executable form
	compiled, err := graph.Compile(gph, graph.NewMessagePregelExecutor())
	if err != nil {
		fmt.Printf("❌ Compilation error: %v\n", err)
		return
	}

	// Execute the graph - routing will happen automatically based on state
	ctx := context.Background()
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			fmt.Printf("❌ Execution error: %v\n", err)
			return
		}
	}

	// Display final state including action history
	finalView, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println("\n  Final state:")
	fmt.Printf("    choice: %v\n", graphstate.GetFromView(finalView, choiceKey))
	fmt.Printf("    next_path: %v\n", graphstate.GetFromView(finalView, nextPathKey))
	fmt.Printf("    action_history: %v\n", graphstate.GetFromView(finalView, actionHistoryKey.Key))
}
