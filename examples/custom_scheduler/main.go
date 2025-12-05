// Package main demonstrates custom scheduler usage in AgentMesh.
// Schedulers control the execution order of vertices within each superstep.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/pregel"
)

// mockGraph implements pregel.Graph interface for demonstration
type mockGraph struct {
	vertices map[string]*mockVertex
}

type mockVertex struct {
	name     string
	outgoing []string
}

func (v *mockVertex) Name() string {
	return v.name
}

func (v *mockVertex) Run(
	ctx context.Context,
	vctx pregel.VertexContext[*mockGraph, string],
	incoming []pregel.Message[string],
) error {
	fmt.Printf("    ⚡ Executing: %s\n", v.name)
	return nil
}

func (g *mockGraph) RootVertices() []string {
	return []string{"high_priority", "medium_priority", "low_priority"}
}

func (g *mockGraph) Outgoing(vertex string) []string {
	if v, ok := g.vertices[vertex]; ok {
		return v.outgoing
	}
	return nil
}

func (g *mockGraph) VertexByName(name string) pregel.Vertex[*mockGraph, string] {
	return g.vertices[name]
}

func (g *mockGraph) State() *mockGraph {
	return g
}

func newMockGraph() *mockGraph {
	vertices := make(map[string]*mockVertex)
	vertices["high_priority"] = &mockVertex{
		name:     "high_priority",
		outgoing: []string{},
	}
	vertices["medium_priority"] = &mockVertex{
		name:     "medium_priority",
		outgoing: []string{},
	}
	vertices["low_priority"] = &mockVertex{
		name:     "low_priority",
		outgoing: []string{},
	}

	return &mockGraph{vertices: vertices}
}

func main() {
	fmt.Println("=== Custom Scheduler Example ===")

	graph := newMockGraph()

	// 1. Default TopologicalScheduler (lexicographic order)
	fmt.Println("1. Default TopologicalScheduler:")
	fmt.Println("   Expected order: high_priority, low_priority, medium_priority (alphabetical)")
	runWithScheduler(graph, nil)

	// 2. PriorityScheduler (high-priority first)
	fmt.Println("\n2. PriorityScheduler (high-priority first):")
	priorities := map[string]int{
		"high_priority":   100,
		"medium_priority": 50,
		"low_priority":    10,
	}
	fmt.Println("   Priorities: high=100, medium=50, low=10")
	fmt.Println("   Expected order: high_priority, medium_priority, low_priority")
	priorityScheduler := pregel.NewPriorityScheduler(priorities, 50)
	runWithScheduler(graph, priorityScheduler)

	// 3. ResourceAwareScheduler (low-cost first)
	fmt.Println("\n3. ResourceAwareScheduler (low-cost first):")
	costs := map[string]int{
		"high_priority":   100, // Expensive
		"medium_priority": 50,  // Medium
		"low_priority":    10,  // Cheap
	}
	fmt.Println("   Costs: high=100, medium=50, low=10")
	fmt.Println("   Expected order: low_priority, medium_priority, high_priority")
	resourceScheduler := pregel.NewResourceAwareScheduler(costs, 50, true)
	runWithScheduler(graph, resourceScheduler)

	fmt.Println("\n=== Example Complete ===")
}

func runWithScheduler(graph *mockGraph, scheduler pregel.Scheduler) {
	// Create runtime with custom scheduler
	var opts []pregel.RuntimeOption[*mockGraph, string]
	if scheduler != nil {
		opts = append(opts, pregel.WithScheduler[*mockGraph, string](scheduler))
	}
	opts = append(opts, pregel.WithMaxWorkers[*mockGraph, string](1)) // Sequential for clear order

	runtime, err := pregel.NewRuntime(graph, opts...)
	if err != nil {
		log.Fatalf("Failed to create runtime: %v", err)
	}

	// Execute runtime
	ctx := context.Background()
	var eventCount int
	for _, err := range runtime.Run(ctx) {
		if err != nil {
			log.Fatalf("Execution error: %v", err)
		}
		eventCount++
	}

	fmt.Printf("   ✓ Completed (%d events)\n", eventCount)
}
