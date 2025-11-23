package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Helper function to repeat strings
func repeatString(char string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += char
	}
	return result
}

func main() {
	// Build a complex graph with conditional routing
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		panic(err)
	}

	// Define state keys
	validKey := state.NewKey("valid", false)
	priorityKey := state.NewKey("priority", "")
	processedKey := state.NewKey("processed", false)
	completeKey := state.NewKey("complete", false)

	// Add nodes
	builder.AddStaticNode("input_validator", graph.NewTargetSet("router"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
		fmt.Println("✓ Validating input...")
		b := graph.NewUpdate()
		graph.UpdateSet(b, validKey, true)
		graph.UpdateSet(b, priorityKey, "high")
		return b.Build()
	})

	builder.AddCommandNode("router", graph.NewTargetSet("high_priority_handler", "normal_handler"), func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
		fmt.Println("✓ Routing request...")
		priority := state.GetFromView(view, priorityKey)
		if priority == "high" {
			return graph.GotoOne("high_priority_handler"), nil
		}
		return graph.GotoOne("normal_handler"), nil
	})

	builder.AddStaticNode("high_priority_handler", graph.NewTargetSet("aggregator"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
		fmt.Println("✓ Handling high priority request...")
		b := graph.NewUpdate()
		graph.UpdateSet(b, processedKey, true)
		return b.Build()
	})

	builder.AddStaticNode("normal_handler", graph.NewTargetSet("aggregator"), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
		fmt.Println("✓ Handling normal request...")
		b := graph.NewUpdate()
		graph.UpdateSet(b, processedKey, true)
		return b.Build()
	})

	builder.AddStaticNode("aggregator", graph.NewTargetSet(graph.EndNode), func(ctx context.Context, view state.ReadView) (state.Updates, error) {
		fmt.Println("✓ Aggregating results...")
		b := graph.NewUpdate()
		graph.UpdateSet(b, completeKey, true)
		return b.Build()
	})

	builder.SetEntryPoint("input_validator")

	// Compile the graph - this is where topology is computed and validated
	runnableGraph, err := builder.Compile()
	if err != nil {
		panic(err)
	}

	fmt.Println("\n" + repeatString("=", 80))
	fmt.Println("GRAPH INTROSPECTION DEMO - Using CompiledGraph Introspection")
	fmt.Println(repeatString("=", 80) + "\n")

	// 1. List all nodes
	fmt.Println("📋 ALL NODES:")
	fmt.Println(repeatString("-", 80))
	nodes := runnableGraph.GetNodes()
	for i, node := range nodes {
		fmt.Printf("%d. %s\n", i+1, node)
	}

	// 2. Get detailed node information
	fmt.Println("\n🔍 NODE DETAILS:")
	fmt.Println(repeatString("-", 80))
	for _, nodeName := range nodes {
		info, err := runnableGraph.GetNodeInfo(nodeName)
		if err != nil {
			continue
		}
		fmt.Printf("\nNode: %s\n", info.Name)
		fmt.Printf("  Type:               %s\n", info.Type)
		fmt.Printf("  Incoming Edges:     %d\n", info.IncomingEdges)
		fmt.Printf("  Outgoing Edges:     %d\n", info.OutgoingEdges)
		fmt.Printf("  Has Retry Policy:   %v\n", info.HasRetryPolicy)
		if info.HasCommandRouting {
			fmt.Printf("  Declared Targets:   %v\n", info.DeclaredTargets)
		}
	}

	// 3. Get topology overview
	fmt.Println("\n🗺️  TOPOLOGY OVERVIEW:")
	fmt.Println(repeatString("-", 80))
	topo := runnableGraph.GetTopology()
	fmt.Printf("Entry Points:      %v\n", topo.EntryPoints)
	fmt.Printf("Exit Points:       %v\n", topo.ExitPoints)
	fmt.Printf("Command Nodes:     %v\n", topo.CommandNodes)
	fmt.Printf("Max Depth:         %d\n", topo.MaxDepth)
	fmt.Printf("Estimated Paths:   %d\n", topo.TotalPaths)

	// 4. Get metrics
	fmt.Println("\n📊 GRAPH METRICS:")
	fmt.Println(repeatString("-", 80))
	metrics := runnableGraph.GetMetrics()
	fmt.Printf("Total Nodes:          %d\n", metrics.TotalNodes)
	fmt.Printf("Total Edges:          %d\n", metrics.TotalEdges)
	fmt.Printf("Average Fan-Out:      %.2f\n", metrics.AverageFanOut)
	fmt.Printf("Max Fan-Out:          %d\n", metrics.MaxFanOut)
	fmt.Printf("Average Fan-In:       %.2f\n", metrics.AverageFanIn)
	fmt.Printf("Max Fan-In:           %d\n", metrics.MaxFanIn)
	fmt.Printf("Cyclomatic Complexity: %d\n", metrics.CyclomaticComplexity)

	// 5. Get dependencies for a specific node
	fmt.Println("\n🔗 DEPENDENCIES (router node):")
	fmt.Println(repeatString("-", 80))
	deps, err := runnableGraph.GetNodeDependencies("router")
	if err == nil {
		fmt.Printf("Direct Predecessors:  %v\n", deps.DirectPredecessors)
		fmt.Printf("Direct Successors:    %v\n", deps.DirectSuccessors)
		fmt.Printf("All Predecessors:     %v\n", deps.AllPredecessors)
		fmt.Printf("All Successors:       %v\n", deps.AllSuccessors)
		fmt.Printf("Depth from START:     %d\n", deps.Depth)
	}

	// 6. Export topology as JSON for external visualization
	fmt.Println("\n📄 JSON EXPORT (Topology):")
	fmt.Println(repeatString("-", 80))
	jsonData, err := json.MarshalIndent(topo, "", "  ")
	if err == nil {
		fmt.Println(string(jsonData))
	}

	// 7. Generate Mermaid flowchart and save to file
	fmt.Println("\n📊 GENERATE MERMAID FLOWCHART:")
	fmt.Println(repeatString("-", 80))

	// Generate flowchart using compiled graph introspection
	flowchart := runnableGraph.MermaidFlowchart("TD")

	// Save to .mmd file in the same directory as main.go
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Printf("Failed to get current file path")
		return
	}
	exampleDir := filepath.Dir(filename)
	outputFile := filepath.Join(exampleDir, "graph.mmd")

	if err := os.WriteFile(outputFile, []byte(flowchart), 0644); err != nil {
		log.Printf("Failed to save flowchart: %v", err)
	} else {
		fmt.Printf("✓ Flowchart saved to: %s\n", outputFile)
		fmt.Println("\nFlowchart preview:")
		fmt.Println(flowchart)
	}

	fmt.Println("\n" + repeatString("=", 80))
	fmt.Println("DEMO COMPLETE")
	fmt.Println(repeatString("=", 80))
}
