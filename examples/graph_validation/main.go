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

	g := graph.New[any, any]()

	// Create a simple linear graph
	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		fmt.Println("Processing...")
		return graph.Cmd().End()
	}, graph.END)

	g.Start("process")

	// Build with default validation
	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Graph built successfully")

	// Execute
	ctx := context.Background()
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("✓ Graph executed successfully")
}

// example2_InvalidGraph demonstrates validation catching errors at compile time
func example2_InvalidGraph() {
	fmt.Println("--- Example 2: Invalid Graph (Caught at Build Time) ---")

	g := graph.New[any, any]()

	// Create an invalid graph - edge to non-existent node
	g.Node("start_node", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().To("non_existent_node")
	}, "non_existent_node") // Node doesn't exist

	g.Start("start_node")

	// Build will fail with validation error
	_, err := g.Build()
	if err != nil {
		fmt.Printf("✓ Validation caught error:\n%v\n\n", err)
	} else {
		fmt.Println("✗ Should have caught validation error")
	}
}

// example3_StrictValidation demonstrates strict validation mode
func example3_StrictValidation() {
	fmt.Println("--- Example 3: Strict Validation ---")

	g := graph.New[any, any]()

	// Create a graph with an unreachable node
	g.Node("reachable", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)

	g.Node("unreachable", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END) // This node has no incoming edges

	g.Start("reachable")

	// Default validation allows unreachable nodes
	_, err := g.Build()
	if err != nil {
		fmt.Printf("✗ Default validation should allow unreachable nodes: %v\n", err)
	} else {
		fmt.Println("✓ Default validation passed (unreachable node allowed)")
	}

	// Create new graph for strict validation test
	g2 := graph.New[any, any]()
	g2.Node("reachable", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)
	g2.Node("unreachable", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().End()
	}, graph.END)
	g2.Start("reachable")

	// Strict validation catches unreachable nodes
	_, err = g2.Build(graph.WithStrictValidation())
	if err != nil {
		fmt.Printf("✓ Strict validation caught unreachable node:\n%v\n\n", err)
	} else {
		fmt.Println("✗ Strict validation should have caught unreachable node")
	}
}

// example4_CustomValidation demonstrates custom validation options
func example4_CustomValidation() {
	fmt.Println("--- Example 4: Custom Validation Options ---")

	// Create a graph with a cycle (for iterative algorithms)
	g := graph.New[any, any]()

	g.Node("agent", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().To("evaluator")
	}, "evaluator")

	g.Node("evaluator", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Check quality and potentially loop back or end
		return graph.Cmd().End()
	}, "agent", graph.END)

	g.Start("agent")

	// Default validation allows cycles
	_, err := g.Build()
	if err != nil {
		fmt.Printf("✗ Default validation should allow cycles: %v\n", err)
	} else {
		fmt.Println("✓ Default validation passed (cycle allowed for iterative pattern)")
	}

	// Create new graph for strict validation test
	g2 := graph.New[any, any]()
	g2.Node("agent", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().To("evaluator")
	}, "evaluator")
	g2.Node("evaluator", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().End()
	}, "agent", graph.END)
	g2.Start("agent")

	// Strict validation rejects cycles
	_, err = g2.Build(graph.WithStrictValidation())
	if err != nil {
		fmt.Printf("✓ Strict validation caught cycle:\n%v\n", err)
	} else {
		fmt.Println("✗ Strict validation should have caught cycle")
	}

	// Custom validation: Allow cycles but reject unreachable nodes
	g3 := graph.New[any, any]()
	g3.Node("agent", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().To("evaluator")
	}, "evaluator")
	g3.Node("evaluator", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		return graph.Cmd().End()
	}, "agent", graph.END)
	g3.Start("agent")

	_, err = g3.Build(graph.WithValidation(graph.ValidationOptions{
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
