// Package main demonstrates type-safe state updates using UpdateBuilder and Append helpers.
// This example shows:
//   - Using UpdateBuilder.Set() for compile-time type safety
//   - Using AppendUpdate[T] and AppendManyUpdates[T] for type-safe list operations
//   - Preventing typos in key names at build time
//   - Type-checked values that match registered key types
//   - Chaining multiple updates with fluent API
//
// Key improvements over raw Updates maps:
//   - Set() provides key name safety through Key[T].Name()
//   - AppendUpdate[T] ensures append values match ListKey[T]
//   - AppendManyUpdates[T] wraps values in SliceOf[T] automatically
//   - Compile errors for type mismatches (not runtime errors)
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/command"
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
	gph.AddNode(&graph.BaseNode{
		NodeName:        "init",
		DeclaredTargets: []string{"process"},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("→ Node: init")

			// Build type-safe updates with fluent API
			// Use SetAll to merge regular keys with list operations
			updates := graphstate.NewUpdateBuilder().
				Set(counterKey, 1).
				Set(statusKey, "initialized").
				Build()

			// Append list values using helper, then merge
			for k, v := range graphstate.AppendUpdate(messagesKey, "System started").Build() {
				updates[k] = v
			}

			// Compile-time type safety examples:
			// .Set(counterKey, "wrong") // ✗ Compiler error: string doesn't match Key[int]
			// AppendUpdate(messagesKey, 123) // ✗ Compiler error: int doesn't match ListKey[string]

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

			// Build updates and merge with list operations
			updates := graphstate.NewUpdateBuilder().
				Set(counterKey, currentCounter+10).
				Set(statusKey, "processing").
				Build()

			// Use AppendManyUpdates for batch list operations
			for k, v := range graphstate.AppendManyUpdates(messagesKey, []string{"Data processed", "Validation complete"}).Build() {
				updates[k] = v
			}

			fmt.Printf("  ✓ Updated counter to %d\n", currentCounter+10)
			return []string{"finalize"}, updates, nil
		},
	})

	// Node 3: Final updates using command.Command
	gph.AddNode(&graph.BaseNode{
		NodeName:        "finalize",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
			fmt.Println("→ Node: finalize")

			// Three equivalent approaches - all type-safe and fluent:

			// Approach 1: With() for method-like syntax
			return command.New().
				Set(statusKey, "finalizing").
				With(func(c *command.Command) *command.Command {
					return command.Append(messagesKey, "Process complete", c)
				}).
				To(graph.EndNode)

			// Approach 2: SetAll() for explicit merge (commented out)
			// return command.New().
			//     Set(statusKey, "finalizing").
			//     SetAll(command.Append(messagesKey, "Process complete")).
			//     To(graph.EndNode)

			// Approach 3: Simple case without mixing (commented out)
			// return command.Append(messagesKey, "Process complete").To(graph.EndNode)
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
	fmt.Println("✓ Type-safe builders prevent wrong value types at compile time")
	fmt.Println("✓ AppendUpdate[T] and AppendManyUpdates[T] enforce ListKey[T] type matching")
	fmt.Println("✓ Fluent API with method chaining for readability")
	fmt.Println("✓ No manual .Name() calls - handled by builder")
	fmt.Println("✓ Automatic SliceOf[T] wrapping for list operations")
	fmt.Println("✓ command.Command and state.UpdateBuilder for different use cases")
}
