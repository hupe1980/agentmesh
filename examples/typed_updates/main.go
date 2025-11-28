// Package main demonstrates type-safe state updates using UpdateBuilder helpers.
// This example shows:
//   - Using UpdateBuilder.With(SetValue()) for compile-time type safety
//   - Using AppendValue[T] for type-safe list operations
//   - Preventing typos in key names at build time
//   - Type-checked values that match registered key types
//   - Chaining multiple updates with fluent API
//
// Key improvements over raw Updates maps:
//   - SetValue() provides compile-time type checking through Key[T]
//   - AppendValue[T] ensures append values match ListKey[T]
//   - Automatic SliceOf[T] wrapping for list operations
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
			updates := graphstate.NewUpdateBuilder().
				With(graphstate.SetValue(counterKey, 1)).
				With(graphstate.SetValue(statusKey, "initialized")).
				With(graphstate.AppendValue(messagesKey, "System started")).
				Build()

			// Compile-time type safety examples:
			// .With(graphstate.SetValue(counterKey, "wrong")) // ✗ Compiler error: string doesn't match Key[int]
			// .With(graphstate.AppendValue(messagesKey, 123)) // ✗ Compiler error: int doesn't match ListKey[string]

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

			// Build updates with list operations
			updates := graphstate.NewUpdateBuilder().
				With(graphstate.SetValue(counterKey, currentCounter+10)).
				With(graphstate.SetValue(statusKey, "processing")).
				With(graphstate.AppendValue(messagesKey, "Data processed", "Validation complete")).
				Build()

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
				With(command.SetValue(statusKey, "finalizing")).
				With(command.Append(messagesKey, "Process complete")).
				To(graph.EndNode)

			// Approach 2: Using separate commands then merging (commented out)
			// statusCmd := command.New().With(command.SetValue(statusKey, "finalizing"))
			// msgCmd := command.New().With(command.Append(messagesKey, "Process complete"))
			// return command.New().With(command.Merge(statusCmd)).With(command.Merge(msgCmd)).To(graph.EndNode)

			// Approach 3: Simple case with just append (commented out)
			// return command.New().With(command.Append(messagesKey, "Process complete")).To(graph.EndNode)
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
	fmt.Println("✓ SetValue[T] and AppendValue[T] enforce Key[T] type matching")
	fmt.Println("✓ Fluent API with With() method for clean chaining")
	fmt.Println("✓ No manual .Name() calls - handled by helpers")
	fmt.Println("✓ Automatic SliceOf[T] wrapping for list operations")
	fmt.Println("✓ command.Command for routing, state.UpdateBuilder for direct updates")
}
