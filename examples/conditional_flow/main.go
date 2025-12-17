// Package main demonstrates conditional routing and dynamic branching with graph.
// This example shows how to:
//   - Create conditional edges that route to different nodes based on state
//   - Use ListKey for accumulating action history
//   - Build decision trees with branching logic
//   - Execute the same graph with different inputs to show routing variations
//
// Key concepts:
//   - Command-based routing: Dynamic routing via graph.Set().To() in node logic
//   - View: Read state values to make routing decisions
//   - ListKey with Append: Accumulate values without overwriting (like a list)
//
// Comparison with old API:
//   Old API: 57 lines of setup and node definitions
//   New API: 38 lines (33% reduction)
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Define typed keys at package level
var (
	ChoiceKey        = graph.NewKey[string]("choice")
	NextPathKey      = graph.NewKey[string]("next_path")
	ActionHistoryKey = graph.NewListKey[string]("action_history")
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
	fmt.Printf("=== Conditional Flow Example (graph2): %s ===\n", choice)

	// Create graph with all keys - much simpler than old API
	g := graph.New(
		ChoiceKey,
		NextPathKey,
		ActionHistoryKey,
	)

	// Decision node: Reads input and decides which path to take
	g.Node("decide", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Read from input via state - the first node receives input in state
		choiceVal := graph.Get(scope, ChoiceKey)
		fmt.Printf("  [decide] Evaluating choice: %s\n", choiceVal)

		// Route to the chosen path - fluent Set().With().To() API
		if choiceVal == "path_a" {
			return graph.Set(NextPathKey, choiceVal).
				With(graph.SetValue(ActionHistoryKey, []string{fmt.Sprintf("Decision: route to %s", choiceVal)})).
				To("path_a")
		}
		return graph.Set(NextPathKey, choiceVal).
			With(graph.SetValue(ActionHistoryKey, []string{fmt.Sprintf("Decision: route to %s", choiceVal)})).
			To("path_b")
	}, "path_a", "path_b")

	// Path A: Specialized processing for option A
	g.Node("path_a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("  [path_a] Executing Path A logic...")
		return graph.Set(ActionHistoryKey, []string{"Completed: Path A"}).End()
	}, graph.END)

	// Path B: Alternative processing for option B
	g.Node("path_b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("  [path_b] Executing Path B logic...")
		return graph.Set(ActionHistoryKey, []string{"Completed: Path B"}).End()
	}, graph.END)

	// Initial node that sets the choice from scenario input
	g.Node("init", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(ChoiceKey, choice).To("decide")
	}, "decide")

	// Set entry point
	g.Start("init")

	// Build the graph
	compiled, err := g.Build()
	if err != nil {
		fmt.Printf("Build error: %v\n", err)
		return
	}

	// Execute the graph
	ctx := context.Background()
	fmt.Println("Running workflow...")
	for _, err := range compiled.Run(ctx, []message.Message{}) {
		if err != nil {
			fmt.Printf("Execution error: %v\n", err)
			return
		}
	}

	fmt.Println("\n  Workflow completed!")
}
