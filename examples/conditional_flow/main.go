// Package main demonstrates conditional routing and dynamic branching in AgentMesh graphs.
// This example shows how to:
//   - Create conditional edges that route to different nodes based on state
//   - Use TopicChannel for accumulating action history
//   - Build decision trees with branching logic
//   - Execute the same graph with different inputs to show routing variations
//
// Key concepts:
//   - AddConditionalEdges: Dynamic routing based on runtime state
//   - Reader: Read state values to make routing decisions
//   - TopicChannel: Accumulate values without overwriting (like a list)
//
// Run: go run main.go

package main

import (
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/graph"
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

	// Initialize graph state with unlimited message history
	state, err := graphstate.NewStateManager(0)
	if err != nil {
		panic(err)
	}
	state.Set("choice", choice)
	state.Set("next_path", "")

	// TopicChannel accumulates values instead of overwriting
	// Perfect for tracking the sequence of actions taken
	state.AddChannel(channel.NewTopicChannel("action_history", 0))

	// Create the graph and helper function for adding nodes
	gph, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}
	mustAddNode := func(n *graph.Node) {
		if err := gph.AddNode(n); err != nil {
			panic(err)
		}
	}

	// Decision node: Reads input and decides which path to take
	mustAddNode(&graph.Node{
		Name: "decide",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			choiceVal, _ := s.Get("choice").(string)
			fmt.Printf("  [decide] Evaluating choice: %s\n", choiceVal)

			// Update state to indicate which path should be taken
			return &graph.NodeResult{
				Updates: map[string]any{
					"next_path": choiceVal,
					"action_history": []string{
						fmt.Sprintf("Decision: route to %s", choiceVal),
					},
				},
			}, nil
		},
	})

	// Path A: Specialized processing for option A
	mustAddNode(&graph.Node{
		Name: "path_a",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("  [path_a] Executing Path A logic...")
			return &graph.NodeResult{
				Updates: map[string]any{
					"action_history": []string{"Completed: Path A"},
				},
			}, nil
		},
	})

	// Path B: Alternative processing for option B
	mustAddNode(&graph.Node{
		Name: "path_b",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("  [path_b] Executing Path B logic...")
			return &graph.NodeResult{
				Updates: map[string]any{
					"action_history": []string{"Completed: Path B"},
				},
			}, nil
		},
	})

	// Build the graph topology with conditional routing
	gph.AddEdge(graph.StartNode, "decide")

	// AddConditionalEdges allows runtime decisions about which nodes to execute next
	// The evaluator function is called at runtime and can return different targets
	// based on the current state
	gph.AddConditionalEdges("decide", func(_ context.Context, s graphstate.Reader) []string {
		// Read the decision from state
		if next, ok := s.Get("next_path").(string); ok && next != "" {
			// Return the selected path as a slice (can return multiple for parallel execution)
			return []string{next}
		}
		// Return empty slice if no valid path
		return nil
	}, []string{"path_a", "path_b"}) // All possible targets must be declared

	// Both paths terminate at END
	gph.AddEdge("path_a", graph.EndNode)
	gph.AddEdge("path_b", graph.EndNode)

	// Compile the graph into executable form
	compiled, err := gph.Compile()
	if err != nil {
		fmt.Printf("❌ Compilation error: %v\n", err)
		return
	}

	// Execute the graph - routing will happen automatically based on state
	if _, err := graph.Last(compiled.Run(context.Background(), nil)); err != nil {
		fmt.Printf("❌ Execution error: %v\n", err)
		return
	}

	// Display final state including action history
	fmt.Println("\n  Final state:")
	finalState := state.GetAll()
	for key, value := range finalState {
		fmt.Printf("    %s: %v\n", key, value)
	}
}
