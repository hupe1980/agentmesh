// Package main demonstrates namespace-based state isolation.
//
// This example shows how to use graph.WithNamespace() to restrict
// which state keys a node can read and write. Nodes wrapped with
// WithNamespace can only access keys within their namespace prefix.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Define keys with namespace prefixes
// Convention: namespace.keyname (e.g., "agent1.counter")
var (
	// Agent 1's private namespace
	agent1Data   = graph.NewKey[string]("agent1.data")
	agent1Status = graph.NewKey[string]("agent1.status")

	// Agent 2's private namespace
	agent2Data   = graph.NewKey[string]("agent2.data")
	agent2Status = graph.NewKey[string]("agent2.status")

	// Global key (no namespace prefix - accessible to all)
	sharedResult = graph.NewKey[string]("result")
)

func main() {
	ctx := context.Background()
	fmt.Println("=== Namespaces Example ===")
	fmt.Println("  Demonstrates state isolation with graph.WithNamespace()")
	fmt.Println()

	// Build graph with all keys
	g := graph.New(agent1Data, agent1Status, agent2Data, agent2Status, sharedResult)

	// Create namespaces for each agent
	ns1 := graph.NewNamespace("agent1")
	ns2 := graph.NewNamespace("agent2")

	// Initialize shared state (no namespace restriction)
	g.Node("init", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("  [init] Setting up initial state")
		return graph.Set(agent1Data, "agent1-initial").
			With(graph.SetValue(agent2Data, "agent2-initial")).
			To("agent1_process", "agent2_process")
	}, "agent1_process", "agent2_process")

	// Agent 1 node - can only access agent1.* keys (and optionally globals)
	g.Node("agent1_process", graph.WithNamespace(func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Can read own namespace
		data := graph.Get(scope, agent1Data)
		fmt.Printf("  [agent1] Read own data: %s\n", data)

		// Cannot read agent2's data (returns zero value)
		otherData := graph.Get(scope, agent2Data)
		fmt.Printf("  [agent1] Tried to read agent2.data: '%s' (empty = blocked)\n", otherData)

		// Can write to own namespace
		return graph.Set(agent1Status, "processed").To("merge")
	}, ns1, false), "merge") // includeGlobal=false

	// Agent 2 node - can only access agent2.* keys (and optionally globals)
	g.Node("agent2_process", graph.WithNamespace(func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Can read own namespace
		data := graph.Get(scope, agent2Data)
		fmt.Printf("  [agent2] Read own data: %s\n", data)

		// Cannot read agent1's data (returns zero value)
		otherData := graph.Get(scope, agent1Data)
		fmt.Printf("  [agent2] Tried to read agent1.data: '%s' (empty = blocked)\n", otherData)

		// Can write to own namespace
		return graph.Set(agent2Status, "processed").To("merge")
	}, ns2, false), "merge") // includeGlobal=false

	// Merge node - can access global result (uses namespace with includeGlobal=true)
	g.Node("merge", graph.WithNamespace(func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Can access global key when includeGlobal=true
		fmt.Println("  [merge] Combining results from both agents")

		// Note: This node uses agent1 namespace but with includeGlobal=true
		// So it can write to the global "result" key
		return graph.Set(sharedResult, "both agents completed").To(graph.END)
	}, ns1, true), graph.END) // includeGlobal=true allows writing to global keys

	g.Start("init")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println()
	fmt.Println("  Namespace features:")
	fmt.Println("    • graph.NewNamespace('prefix') - create a namespace")
	fmt.Println("    • graph.WithNamespace(fn, ns, includeGlobal) - restrict node access")
	fmt.Println("    • Keys with 'prefix.' are in the namespace")
	fmt.Println("    • Keys without dots are global (if includeGlobal=true)")
	fmt.Println("    • Violations return zero values for reads, error for writes")
}
