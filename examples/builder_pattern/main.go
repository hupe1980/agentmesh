package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// This example demonstrates the new builder pattern for state management.
// The builder pattern enforces compile-time separation between setup and runtime phases.

func main() {
	ctx := context.Background()

	// Define state keys
	counterKey := state.NewKey("counter", 0)
	textKey := state.NewKey("text", "")

	fmt.Println("=== State Manager Builder Pattern ===")
	fmt.Println()

	// NEW PATTERN: Use ManagerBuilder for type-safe setup
	fmt.Println("1. Create ManagerBuilder (mutable setup phase)")
	stateBuilder := state.NewManagerBuilder()

	// Register keys during mutable setup phase
	fmt.Println("2. Register keys during setup")
	if err := state.RegisterKey(stateBuilder, counterKey); err != nil {
		log.Fatal(err)
	}

	if err := state.RegisterKey(stateBuilder, textKey); err != nil {
		log.Fatal(err)
	}

	// Build returns frozen Manager - schema cannot be modified after this
	fmt.Println("3. Build() returns frozen Manager")
	mgr := stateBuilder.Build()
	fmt.Println("   ✓ Manager is now immutable - cannot register more keys")
	fmt.Println()

	// Create simple graph with the pre-configured manager
	fmt.Println("4. Create graph with frozen manager")
	graphBuilder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		log.Fatal(err)
	}

	// Add nodes using the current graph builder API
	graphBuilder.SetEntryPoint("node1")

	graphBuilder.AddNodeFunc("node1", []string{"node2"}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
		fmt.Println("\n[node1] Setting initial values")
		return []string{"node2"}, state.NewUpdateBuilder().
			With(state.SetValue(counterKey, 42)).
			With(state.SetValue(textKey, "Hello from node1")).
			Build(), nil
	})

	graphBuilder.AddNodeFunc("node2", []string{graph.EndNode}, func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
		// Read current state using type-safe accessors
		counter := state.GetFromView(view, counterKey)
		text := state.GetFromView(view, textKey)

		fmt.Printf("[node2] Reading state:\n")
		fmt.Printf("  Counter: %d\n", counter)
		fmt.Printf("  Text: %s\n", text)

		// Update state
		return []string{graph.EndNode}, state.NewUpdateBuilder().
			With(state.SetValue(counterKey, counter+1)).
			With(state.SetValue(textKey, text+" -> node2 added")).
			Build(), nil
	})

	// Compile graph
	fmt.Println("\n5. Compile graph")
	compiled, err := graphBuilder.Compile()
	if err != nil {
		log.Fatal(err)
	}

	// Execute
	fmt.Println("6. Execute graph")
	for _, err := range compiled.Run(ctx, []message.Message{}) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println()

	// Demonstrate immutability
	fmt.Println("\n=== Immutability Enforcement ===")
	fmt.Println("✓ Builder pattern enforces immutability at compile-time")
	fmt.Println("✓ Cannot register keys after Build()")
	fmt.Println("✓ Attempting to call RegisterKey(mgr, ...) would cause compile error:")
	fmt.Println("  Error: 'cannot use mgr (variable of type *state.Manager) as *state.ManagerBuilder value'")

	// This would be a compile error if uncommented:
	// newKey := state.NewKey("new_key", "")
	// state.RegisterKey(mgr, newKey)  // ERROR: mgr is *Manager, not *ManagerBuilder

	fmt.Println("\n✅ Example completed successfully")
}
