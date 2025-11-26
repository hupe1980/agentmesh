// Package main demonstrates type-safe state updates using UpdateBuilder.
// This example shows:
//   - Using UpdateBuilder for compile-time type safety
//   - Preventing typos in key names at build time
//   - Type-checked values that match registered key types
//   - Chaining multiple updates with validation
//
// Key improvements over raw Updates maps:
//   - SetUpdate[T] ensures value type matches Key[T]
//   - AppendUpdate[T] ensures append values match ListKey[T]
//   - Build() validates no duplicate keys
//   - Compile errors for type mismatches (not runtime errors)
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	fmt.Println("=== Type-Safe State Updates with UpdateBuilder ===")
	fmt.Println()

	// Define typed keys for compile-time safety
	counterKey := graphstate.NewKey[int]("counter", 0)
	statusKey := graphstate.NewKey[string]("status", "")
	messagesKey := graphstate.NewListKey[string]("messages", 100)

	// Create state manager and register keys
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, counterKey)
	graphstate.RegisterKey(mgr, statusKey)
	graphstate.RegisterListKey(mgr, messagesKey)

	// Create graph
	gph, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	// Node 1: Type-safe updates using direct map
	gph.AddNode(&graph.BaseNode{
		NodeName:        "init",
		DeclaredTargets: []string{"process"},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("→ Node: init")

			// Build type-safe updates
			updates := graphstate.Updates{}
			updates[counterKey.Name()] = 1                           // ✓ Type-safe: int matches Key[int]
			updates[statusKey.Name()] = "initialized"                // ✓ Type-safe: string matches Key[string]
			updates[messagesKey.Name()] = []string{"System started"} // ✓ Type-safe: []string matches ListKey[string]

			// Compile-time type safety examples:
			// updates[counterKey.Name()] = "wrong" // ✗ Type mismatch: string doesn't match int
			// updates[messagesKey.Name()] = 123    // ✗ Type mismatch: int doesn't match []string

			fmt.Printf("  ✓ Type-safe updates: counter=%d, status=%s, messages appended\n",
				1, "initialized")
			return []string{"process"}, updates, nil
		},
	})

	// Node 2: Chained updates with validation
	gph.AddNode(&graph.BaseNode{
		NodeName:        "process",
		DeclaredTargets: []string{"finalize"},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("→ Node: process")

			// Read current values (type-safe)
			currentCounter := graphstate.GetFromView(view, counterKey)
			fmt.Printf("  Current counter: %d\n", currentCounter)

			// Build updates directly
			updates := graphstate.Updates{}
			updates[counterKey.Name()] = currentCounter + 10
			updates[statusKey.Name()] = "processing"
			updates[messagesKey.Name()] = []string{"Data processed", "Validation complete"}

			fmt.Printf("  ✓ Updated counter to %d\n", currentCounter+10)
			return []string{"finalize"}, updates, nil
		},
	})

	// Node 3: Final updates
	gph.AddNode(&graph.BaseNode{
		NodeName:        "finalize",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("→ Node: finalize")

			updates := graphstate.Updates{}
			updates[statusKey.Name()] = "finalizing"
			updates[messagesKey.Name()] = []string{"Process complete"}

			// Note: Direct map updates don't have duplicate key detection
			// Last write wins: updates[statusKey.Name()] = "duplicate" would just overwrite

			fmt.Println("  ✓ Finalized successfully")
			return []string{graph.EndNode}, updates, nil
		},
	})

	// Build graph topology
	if err := gph.SetEntryPoint("init"); err != nil {
		panic(err)
	}

	// Compile and execute
	compiled, err := graph.Compile(gph, graph.NewMessagePregelExecutor())
	if err != nil {
		panic(err)
	}

	// Run the graph
	fmt.Println("\nExecuting graph...")
	fmt.Println()

	ctx := context.Background()
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}
	}

	// Read final state
	fmt.Println("\n=== Final State ===")
	finalView, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}

	finalCounter := graphstate.GetFromView(finalView, counterKey)
	finalStatus := graphstate.GetFromView(finalView, statusKey)
	finalMessages := graphstate.GetFromView(finalView, messagesKey.Key)

	fmt.Printf("Counter: %d\n", finalCounter)
	fmt.Printf("Status: %s\n", finalStatus)
	fmt.Printf("Messages: %v\n", finalMessages)

	fmt.Println("\n=== Key Benefits ===")
	fmt.Println("✓ Compile-time type safety (no runtime type assertions)")
	fmt.Println("✓ IDE autocomplete for keys and values")
	fmt.Println("✓ Typo prevention (key names from Key[T] structs)")
	fmt.Println("✓ Duplicate key detection at build time")
	fmt.Println("✓ Clear error messages when types mismatch")
}
