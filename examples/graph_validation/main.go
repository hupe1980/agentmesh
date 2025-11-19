// Graph Validation Example
//
// This example demonstrates AgentMesh's comprehensive graph validation layer
// that catches errors before runtime.

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	fmt.Println("=== Graph Validation Examples ===")

	// Example 1: Valid graph compiles successfully
	example1_ValidGraph()

	// Example 2: Invalid graph caught at compile time
	example2_InvalidGraph()

	// Example 3: Strict validation
	example3_StrictValidation()

	// Example 4: Custom validation options
	example4_CustomValidation()
}

// example1_ValidGraph demonstrates a valid graph that compiles successfully
func example1_ValidGraph() {
	fmt.Println("--- Example 1: Valid Graph ---")

	mgr := state.NewManager()
	g, _ := graph.NewGraph(mgr)

	// Create a simple linear graph
	g.AddNode(graph.NewBaseNode("process",
		func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			fmt.Println("Processing...")
			return nil, nil
		},
	))

	g.AddEdge(graph.StartNode, "process")
	g.AddEdge("process", graph.EndNode)

	// Compile with default validation
	runnable, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Graph compiled successfully")

	// Execute
	ctx := context.Background()
	for _, err := range runnable.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("✓ Graph executed successfully")
}

// example2_InvalidGraph demonstrates validation catching errors at compile time
func example2_InvalidGraph() {
	fmt.Println("--- Example 2: Invalid Graph (Caught at Compile Time) ---")

	mgr := state.NewManager()
	g, _ := graph.NewGraph(mgr)

	// Create an invalid graph - edge to non-existent node
	g.AddNode(graph.NewBaseNode("start_node",
		func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		},
	))

	// This edge references a node that doesn't exist
	g.AddEdge("start_node", "non_existent_node")

	// Compilation will fail with validation error
	_, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		fmt.Printf("✓ Validation caught error:\n%v\n\n", err)
	} else {
		fmt.Println("✗ Should have caught validation error")
	}
}

// example3_StrictValidation demonstrates strict validation mode
func example3_StrictValidation() {
	fmt.Println("--- Example 3: Strict Validation ---")

	mgr := state.NewManager()
	g, _ := graph.NewGraph(mgr)

	// Create a graph with an unreachable node
	g.AddNode(graph.NewBaseNode("reachable",
		func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		},
	))
	g.AddNode(graph.NewBaseNode("unreachable",
		func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		},
	))

	g.AddEdge(graph.StartNode, "reachable")
	g.AddEdge("reachable", graph.EndNode)
	// "unreachable" has no incoming edges

	// Default validation allows unreachable nodes
	_, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		fmt.Printf("✗ Default validation should allow unreachable nodes: %v\n", err)
	} else {
		fmt.Println("✓ Default validation passed (unreachable node allowed)")
	}

	// Strict validation catches unreachable nodes
	_, err = graph.Compile(g, graph.NewMessagePregelExecutor(), graph.WithStrictValidation())
	if err != nil {
		fmt.Printf("✓ Strict validation caught unreachable node:\n%v\n\n", err)
	} else {
		fmt.Println("✗ Strict validation should have caught unreachable node")
	}
}

// example4_CustomValidation demonstrates custom validation options
func example4_CustomValidation() {
	fmt.Println("--- Example 4: Custom Validation Options ---")

	mgr := state.NewManager()
	g, _ := graph.NewGraph(mgr)

	// Create a graph with a cycle (for iterative algorithms)
	g.AddNode(graph.NewBaseNode("agent",
		func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			return nil, nil
		},
	))
	g.AddNode(graph.NewBaseNode("evaluator",
		func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
			// Check quality and potentially loop back
			return nil, nil
		},
	))

	g.AddEdge(graph.StartNode, "agent")
	g.AddEdge("agent", "evaluator")
	g.AddEdge("evaluator", "agent") // Creates a cycle for refinement
	g.AddEdge("evaluator", graph.EndNode)

	// Default validation allows cycles
	_, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		fmt.Printf("✗ Default validation should allow cycles: %v\n", err)
	} else {
		fmt.Println("✓ Default validation passed (cycle allowed for iterative pattern)")
	}

	// Strict validation rejects cycles
	_, err = graph.Compile(g, graph.NewMessagePregelExecutor(), graph.WithStrictValidation())
	if err != nil {
		fmt.Printf("✓ Strict validation caught cycle:\n%v\n", err)
	} else {
		fmt.Println("✗ Strict validation should have caught cycle")
	}

	// Custom validation: Allow cycles but reject unreachable nodes
	_, err = graph.Compile(g, graph.NewMessagePregelExecutor(), graph.WithValidation(graph.ValidationOptions{
		AllowCycles:            true,  // Cycles OK for iterative algorithms
		AllowDisconnectedNodes: false, // But all nodes must be reachable
	}))
	if err != nil {
		fmt.Printf("✗ Custom validation failed: %v\n", err)
	} else {
		fmt.Println("✓ Custom validation passed (cycles allowed, reachability enforced)")
	}

	fmt.Println("\n=== Validation Examples Complete ===")
}
