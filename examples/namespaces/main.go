package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// This example demonstrates the namespace system for state isolation.
// Namespaces allow different components (agents, subgraphs, tools) to
// have their own isolated state while sharing the same state manager.

func main() {
	ctx := context.Background()
	mgr := state.NewManager()

	// 1. Global Keys (Default) - Simple, no prefix
	// Use global keys when you don't need isolation
	var GlobalConfig = state.NewKey[string]("config", "")
	var GlobalCounter = state.NewKey[int]("counter", 0)

	if err := state.RegisterKey(mgr, GlobalConfig); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterKey(mgr, GlobalCounter); err != nil {
		log.Fatal(err)
	}

	// Set global values
	if err := state.Set(ctx, mgr, GlobalConfig, "production"); err != nil {
		log.Fatal(err)
	}
	if err := state.Set(ctx, mgr, GlobalCounter, 100); err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Global Keys (Default) ===")
	config, _ := state.Get(ctx, mgr, GlobalConfig)
	counter, _ := state.Get(ctx, mgr, GlobalCounter)
	fmt.Printf("Config: %s\n", config)
	fmt.Printf("Counter: %d\n", counter)
	fmt.Println()

	// 2. Namespaced Keys - Use when you need isolation
	// Create namespaces for different agents
	agent1NS := state.MustNamespace("agent1")
	agent2NS := state.MustNamespace("agent2")

	// Create namespaced keys with dot notation: "agent1.status"
	agent1Status := state.TypedKey[string](agent1NS, "status", "")
	agent1Progress := state.TypedKey[int](agent1NS, "progress", 0)
	agent2Status := state.TypedKey[string](agent2NS, "status", "")
	agent2Progress := state.TypedKey[int](agent2NS, "progress", 0)

	// Register namespaced keys
	if err := state.RegisterKey(mgr, agent1Status); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterKey(mgr, agent1Progress); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterKey(mgr, agent2Status); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterKey(mgr, agent2Progress); err != nil {
		log.Fatal(err)
	}

	// Set values - no collisions even with same key names
	if err := state.Set(ctx, mgr, agent1Status, "processing"); err != nil {
		log.Fatal(err)
	}
	if err := state.Set(ctx, mgr, agent1Progress, 50); err != nil {
		log.Fatal(err)
	}
	if err := state.Set(ctx, mgr, agent2Status, "waiting"); err != nil {
		log.Fatal(err)
	}
	if err := state.Set(ctx, mgr, agent2Progress, 0); err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Namespaced Keys (Isolation) ===")
	a1Status, _ := state.Get(ctx, mgr, agent1Status)
	a1Progress, _ := state.Get(ctx, mgr, agent1Progress)
	a2Status, _ := state.Get(ctx, mgr, agent2Status)
	a2Progress, _ := state.Get(ctx, mgr, agent2Progress)
	fmt.Printf("Agent1 - Status: %s, Progress: %d%%\n", a1Status, a1Progress)
	fmt.Printf("Agent2 - Status: %s, Progress: %d%%\n", a2Status, a2Progress)
	fmt.Println()

	// 3. Namespace Operations
	// Get a view of all keys in a specific namespace
	view, err := mgr.CreateReadView(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Namespace Views ===")
	agent1View := state.GetNamespaceView(view, agent1NS)
	fmt.Printf("Agent1 namespace keys: %v\n", agent1View)

	globalView := state.GetNamespaceView(view, state.Global)
	fmt.Printf("Global namespace keys: %v\n", globalView)
	fmt.Println()

	// 4. List all namespaces
	fmt.Println("=== Active Namespaces ===")
	namespaces := state.ListNamespaces(view)
	for _, ns := range namespaces {
		if ns.IsGlobal() {
			fmt.Println("- (global)")
		} else {
			fmt.Printf("- %s\n", ns.Name())
		}
	}
	fmt.Println()

	// 5. Copy namespace (useful for subgraph handoffs)
	agent3NS := state.MustNamespace("agent3")
	agent3Status := state.TypedKey[string](agent3NS, "status", "")
	agent3Progress := state.TypedKey[int](agent3NS, "progress", 0)

	// Must register target keys before copying
	if err := state.RegisterKey(mgr, agent3Status); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterKey(mgr, agent3Progress); err != nil {
		log.Fatal(err)
	}

	// Copy agent1 state to agent3
	if err := state.CopyNamespace(ctx, mgr, agent1NS, agent3NS); err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Namespace Copy (agent1 -> agent3) ===")
	a3Status, _ := state.Get(ctx, mgr, agent3Status)
	a3Progress, _ := state.Get(ctx, mgr, agent3Progress)
	fmt.Printf("Agent3 - Status: %s, Progress: %d%% (copied from Agent1)\n", a3Status, a3Progress)
	fmt.Println()

	// 6. Key introspection
	fmt.Println("=== Key Introspection ===")
	fmt.Printf("agent1Status key name: %s\n", agent1Status.Name())
	fmt.Printf("Is namespaced? %v\n", state.IsNamespaced(agent1Status.Name()))

	nsName, localName := state.ParseNamespacedKey(agent1Status.Name())
	fmt.Printf("Namespace: %s, Local name: %s\n", nsName, localName)
	fmt.Println()

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Println("✓ Global keys: Simple, no prefix (e.g., 'config', 'counter')")
	fmt.Println("✓ Namespaced keys: Isolated with dot notation (e.g., 'agent1.status')")
	fmt.Println("✓ Zero overhead: Just string prefixes, full type safety")
	fmt.Println("✓ Operations: GetNamespaceView, ListNamespaces, CopyNamespace")
}
