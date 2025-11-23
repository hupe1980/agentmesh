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
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "process",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			fmt.Println("Processing...")
			return graph.End(nil), nil
		},
	})

	if err := g.SetEntryPoint("process"); err != nil {
		panic(err)
	}

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
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "start_node",
		DeclaredTargets: graph.NewTargetSet("non_existent_node"),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			return graph.GotoOne("non_existent_node"), nil
		},
	})

	// This edge references a node that doesn't exist
	// Removed invalid edge to demonstrate validation

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
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "reachable",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			return graph.End(nil), nil
		},
	})
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "unreachable",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			return graph.End(nil), nil
		},
	})

	if err := g.SetEntryPoint("reachable"); err != nil {
		panic(err)
	}
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
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "agent",
		DeclaredTargets: graph.NewTargetSet("evaluator"),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			return graph.GotoOne("evaluator"), nil
		},
	})
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "evaluator",
		DeclaredTargets: graph.NewTargetSet("agent", graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			// Check quality and potentially loop back
			return graph.End(nil), nil
		},
	})

	if err := g.SetEntryPoint("agent"); err != nil {
		panic(err)
	}
	// Note: Cycle is created via Command pattern routing in node logic

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
