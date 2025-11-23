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

	// Node 1: Type-safe updates using UpdateBuilder
	gph.AddNode(&graph.BaseCommandNode{
		NodeName:        "init",
		DeclaredTargets: graph.NewTargetSet("process"),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("→ Node: init")

			// Build type-safe updates
			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, counterKey, 1)                    // ✓ Type-safe: int matches Key[int]
			graphstate.SetUpdate(builder, statusKey, "initialized")         // ✓ Type-safe: string matches Key[string]
			graphstate.AppendUpdate(builder, messagesKey, "System started") // ✓ Type-safe: string matches ListKey[string]

			// Compile-time type safety examples:
			// graphstate.SetUpdate(builder, counterKey, "wrong") // ✗ Compile error: string doesn't match Key[int]
			// graphstate.AppendUpdate(builder, messagesKey, 123) // ✗ Compile error: int doesn't match ListKey[string]

			updates, err := builder.Build()
			if err != nil {
				return nil, fmt.Errorf("build failed: %w", err)
			}

			fmt.Printf("  ✓ Type-safe updates: counter=%d, status=%s, messages appended\n",
				1, "initialized")
			return graph.Goto("process", updates), nil
		},
	})

	// Node 2: Chained updates with validation
	gph.AddNode(&graph.BaseCommandNode{
		NodeName:        "process",
		DeclaredTargets: graph.NewTargetSet("finalize"),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("→ Node: process")

			// Read current values (type-safe)
			currentCounter := graphstate.GetFromView(view, counterKey)
			fmt.Printf("  Current counter: %d\n", currentCounter)

			// Build updates with chaining
			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, counterKey, currentCounter+10)
			graphstate.SetUpdate(builder, statusKey, "processing")
			graphstate.AppendUpdate(builder, messagesKey, "Data processed", "Validation complete")

			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}

			fmt.Printf("  ✓ Updated counter to %d\n", currentCounter+10)
			return graph.Goto("finalize", updates), nil
		},
	})

	// Node 3: Demonstrate duplicate key detection
	gph.AddNode(&graph.BaseCommandNode{
		NodeName:        "finalize",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("→ Node: finalize")

			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, statusKey, "finalizing")
			graphstate.AppendUpdate(builder, messagesKey, "Process complete")

			// Example of error handling for duplicate keys
			// graphstate.SetUpdate(builder, statusKey, "duplicate") // Would cause Build() to return error

			updates, err := builder.Build()
			if err != nil {
				return nil, fmt.Errorf("duplicate key error: %w", err)
			}

			fmt.Println("  ✓ Finalized successfully")
			return graph.End(updates), nil
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
