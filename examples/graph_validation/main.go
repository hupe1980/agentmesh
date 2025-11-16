// Graph Validation Example
//
// This example demonstrates AgentMesh's comprehensive graph validation layer
// that catches errors before runtime.

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/exec"
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

	stateManager, _ := state.NewStateManager(0)
	g, _ := graph.NewGraph(stateManager)

	// Create a simple linear graph
	g.AddNode(&graph.Node{
		Name: "process",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			fmt.Println("Processing...")
			return &graph.NodeResult{}, nil
		},
	})

	g.AddEdge(compile.StartNode, "process")
	g.AddEdge("process", compile.EndNode)

	// Compile with default validation
	runnable, err := exec.CompileGraph(g)
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

	stateManager, _ := state.NewStateManager(0)
	g, _ := graph.NewGraph(stateManager)

	// Create an invalid graph - edge to non-existent node
	g.AddNode(&graph.Node{
		Name: "start_node",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})

	// This edge references a node that doesn't exist
	g.AddEdge("start_node", "non_existent_node")

	// Compilation will fail with validation error
	_, err := exec.CompileGraph(g)
	if err != nil {
		fmt.Printf("✓ Validation caught error:\n%v\n\n", err)
	} else {
		fmt.Println("✗ Should have caught validation error")
	}
}

// example3_StrictValidation demonstrates strict validation mode
func example3_StrictValidation() {
	fmt.Println("--- Example 3: Strict Validation ---")

	stateManager, _ := state.NewStateManager(0)
	g, _ := graph.NewGraph(stateManager)

	// Create a graph with an unreachable node
	g.AddNode(&graph.Node{
		Name: "reachable",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})
	g.AddNode(&graph.Node{
		Name: "unreachable",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})

	g.AddEdge(compile.StartNode, "reachable")
	g.AddEdge("reachable", compile.EndNode)
	// "unreachable" has no incoming edges

	// Default validation allows unreachable nodes
	_, err := exec.CompileGraph(g)
	if err != nil {
		fmt.Printf("✗ Default validation should allow unreachable nodes: %v\n", err)
	} else {
		fmt.Println("✓ Default validation passed (unreachable node allowed)")
	}

	// Strict validation catches unreachable nodes
	_, err = exec.CompileGraph(g, exec.WithStrictValidation())
	if err != nil {
		fmt.Printf("✓ Strict validation caught unreachable node:\n%v\n\n", err)
	} else {
		fmt.Println("✗ Strict validation should have caught unreachable node")
	}
}

// example4_CustomValidation demonstrates custom validation options
func example4_CustomValidation() {
	fmt.Println("--- Example 4: Custom Validation Options ---")

	stateManager, _ := state.NewStateManager(0)
	g, _ := graph.NewGraph(stateManager)

	// Create a graph with a cycle (for iterative algorithms)
	g.AddNode(&graph.Node{
		Name: "agent",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		},
	})
	g.AddNode(&graph.Node{
		Name: "evaluator",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			// Check quality and potentially loop back
			return &graph.NodeResult{}, nil
		},
	})

	g.AddEdge(compile.StartNode, "agent")
	g.AddEdge("agent", "evaluator")
	g.AddEdge("evaluator", "agent") // Creates a cycle for refinement
	g.AddEdge("evaluator", compile.EndNode)

	// Default validation allows cycles
	_, err := exec.CompileGraph(g)
	if err != nil {
		fmt.Printf("✗ Default validation should allow cycles: %v\n", err)
	} else {
		fmt.Println("✓ Default validation passed (cycle allowed for iterative pattern)")
	}

	// Strict validation rejects cycles
	_, err = exec.CompileGraph(g, exec.WithStrictValidation())
	if err != nil {
		fmt.Printf("✓ Strict validation caught cycle:\n%v\n", err)
	} else {
		fmt.Println("✗ Strict validation should have caught cycle")
	}

	// Custom validation: Allow cycles but reject unreachable nodes
	_, err = exec.CompileGraph(g, exec.WithValidation(compile.ValidationOptions{
		AllowCycles:      true,  // Cycles OK for iterative algorithms
		AllowUnreachable: false, // But all nodes must be reachable
		AllowDeadEnds:    false, // And all nodes must reach END
	}))
	if err != nil {
		fmt.Printf("✗ Custom validation failed: %v\n", err)
	} else {
		fmt.Println("✓ Custom validation passed (cycles allowed, reachability enforced)")
	}

	fmt.Println("\n=== Validation Examples Complete ===")
}
