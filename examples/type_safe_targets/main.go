// Type-Safe Targets Example demonstrates Phase 5 of the Command pattern implementation.
// It shows how to use TargetSet for compile-time validation of routing targets.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	// Build a graph with type-safe targets
	_, err := buildTypeSafeGraph()
	if err != nil {
		log.Fatalf("Failed to build graph: %v", err)
	}

	fmt.Println("Type-safe targets demonstration:")
	fmt.Println("- TargetSet ensures all routing targets are declared upfront")
	fmt.Println("- targets.Get() provides safe access to target names")
	fmt.Println("- Typos in target names return empty string, caught at runtime")
	fmt.Println("- IDE autocomplete shows all available targets")
	fmt.Println("\nGraph compiled successfully with type-safe routing!")
}

func buildTypeSafeGraph() (*graph.Compiled[[]message.Message, message.Message], error) {
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		return nil, err
	}

	// Register messages key
	messagesKey := state.NewListKey[message.Message]("__messages__", 0)
	if err := state.RegisterListKey(builder.Manager(), messagesKey); err != nil {
		return nil, err
	}

	// Define type-safe target set for the router node
	// This provides compile-time validation of routing targets
	routerTargets := graph.NewTargetSet(
		"validation",
		"processing",
		graph.EndNode,
	)

	// Router node with type-safe targets
	builder.AddCommandNode("router", routerTargets,
		func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			// Type-safe routing - if we typo a target name, targets.Get() returns ""
			// which will fail at runtime with a clear error

			// Demonstrate type-safe target access
			validationTarget := routerTargets.Get("validation")
			processingTarget := routerTargets.Get("processing")

			// This would return "" since "typo" doesn't exist:
			// wrongTarget := routerTargets.Get("typo")

			fmt.Println("  -> Router: Checking targets...")
			fmt.Printf("     Validation target: %s\n", validationTarget)
			fmt.Printf("     Processing target: %s\n", processingTarget)

			// Route to validation first
			return routerTargets.Goto(
				validationTarget,
				state.Updates{},
			), nil
		},
	)

	// Validation node targets
	validationTargets := graph.NewTargetSet(
		"processing",
		graph.EndNode,
	)

	builder.AddCommandNode("validation", validationTargets,
		func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			fmt.Println("  -> Validation: Passed")

			// Route to processing using type-safe target
			return validationTargets.Goto(
				validationTargets.Get("processing"),
				state.Updates{"validated": true},
			), nil
		},
	)

	// Processing node - simple static routing
	processingTargets := graph.NewTargetSet(graph.EndNode)

	builder.AddCommandNode("processing", processingTargets,
		func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			fmt.Println("  -> Processing: Complete")

			// End execution by routing to EndNode
			return processingTargets.Goto(
				processingTargets.Get(graph.EndNode),
				state.Updates{"processed": true},
			), nil
		},
	)

	return builder.
		SetEntryPoint("router").
		Compile()
}

// Example of using typed command builder with fluent API
func exampleFluentAPI() error {
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		return err
	}

	// Register messages key
	messagesKey := state.NewListKey[message.Message]("__messages__", 0)
	if err := state.RegisterListKey(builder.Manager(), messagesKey); err != nil {
		return err
	}

	// Create target set
	targets := graph.NewTargetSet("next", "error", graph.EndNode)

	// Use fluent API with type-safe targets
	builder.AddCommandNode("example", targets,
		func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			// Type-safe routing with Goto
			hasError := false
			if hasError {
				return targets.Goto(targets.Get("error"), state.Updates{"error": "something failed"}), nil
			}

			// Or route to EndNode
			return targets.Goto(targets.Get(graph.EndNode), state.Updates{"done": true}), nil
		},
	)

	_, err = builder.Compile()
	return err
}

// Example of custom MustGet helper for panicking on missing targets
func exampleMustGet() {
	targets := graph.NewTargetSet("valid_target", graph.EndNode)

	// Create a MustGet helper that panics if target doesn't exist
	mustGet := func(ts *graph.TargetSet, name string) string {
		if !ts.Has(name) {
			panic(fmt.Sprintf("target not found: %s", name))
		}
		return ts.Get(name)
	}

	// This works fine
	validTarget := mustGet(targets, "valid_target")
	fmt.Println("Got target:", validTarget)

	// This would panic at runtime
	// invalidTarget := mustGet(targets, "typo_target") // panic!
}
