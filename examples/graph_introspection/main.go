// Package main demonstrates graph introspection capabilities.
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
)

// Helper function to repeat strings
func repeatString(char string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += char
	}
	return result
}

// Define state keys
var (
	validKey     = graph.NewKey[bool]("valid")
	priorityKey  = graph.NewKey[string]("priority")
	processedKey = graph.NewKey[bool]("processed")
	completeKey  = graph.NewKey[bool]("complete")
)

func main() {
	// Build a complex graph with conditional routing
	g := graph.New(validKey, priorityKey, processedKey, completeKey)

	// Add nodes using the new builder API
	g.Node("input_validator", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("✓ Validating input...")
		return graph.Set(validKey, true).
			With(graph.SetValue(priorityKey, "high")).
			To("router")
	}, "router")

	g.Node("router", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("✓ Routing request...")
		priority := graph.Get(scope, priorityKey)
		if priority == "high" {
			return graph.To("high_priority_handler")
		}
		return graph.To("normal_handler")
	}, "high_priority_handler", "normal_handler")

	g.Node("high_priority_handler", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("✓ Handling high priority request...")
		return graph.Set(processedKey, true).To("aggregator")
	}, "aggregator")

	g.Node("normal_handler", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("✓ Handling normal request...")
		return graph.Set(processedKey, true).To("aggregator")
	}, "aggregator")

	g.Node("aggregator", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("✓ Aggregating results...")
		return graph.Set(completeKey, true).End()
	}, graph.END)

	// Set entry point
	g.Start("input_validator")

	// Build the graph to get the compiled version for introspection
	compiled, err := g.Build()
	if err != nil {
		log.Fatalf("Failed to build graph: %v", err)
	}

	fmt.Println("\n" + repeatString("=", 80))
	fmt.Println("GRAPH INTROSPECTION DEMO")
	fmt.Println(repeatString("=", 80) + "\n")

	// 1. List all nodes
	fmt.Println("📋 ALL NODES:")
	fmt.Println(repeatString("-", 80))
	nodes := compiled.GetNodes()
	for i, node := range nodes {
		fmt.Printf("%d. %s\n", i+1, node)
	}

	// 2. Get detailed node information
	fmt.Println("\n🔍 NODE DETAILS:")
	fmt.Println(repeatString("-", 80))
	for _, nodeName := range nodes {
		info, err := compiled.GetNodeInfo(nodeName)
		if err != nil {
			continue
		}
		fmt.Printf("\nNode: %s\n", info.Name)
		fmt.Printf("  Type:               %s\n", info.Type)
		fmt.Printf("  Incoming Edges:     %d\n", info.IncomingEdges)
		fmt.Printf("  Outgoing Edges:     %d\n", info.OutgoingEdges)
		fmt.Printf("  Is Entry Point:     %v\n", info.IsEntryPoint)
		fmt.Printf("  Has Interrupt:      %v\n", info.HasInterrupt)
		fmt.Printf("  Declared Targets:   %v\n", info.DeclaredTargets)
	}

	// 3. Get topology overview
	fmt.Println("\n🗺️  TOPOLOGY OVERVIEW:")
	fmt.Println(repeatString("-", 80))
	topo := compiled.GetTopology()
	fmt.Printf("Entry Points:      %v\n", topo.EntryPoints)
	fmt.Printf("Exit Points:       %v\n", topo.ExitPoints)
	fmt.Printf("Total Nodes:       %d\n", len(topo.Nodes))
	fmt.Printf("Total Edges:       %d\n", len(topo.Edges))

	// 4. Get metrics
	fmt.Println("\n📊 GRAPH METRICS:")
	fmt.Println(repeatString("-", 80))
	metrics := compiled.GetMetrics()
	fmt.Printf("Total Nodes:          %d\n", metrics.TotalNodes)
	fmt.Printf("Total Edges:          %d\n", metrics.TotalEdges)
	fmt.Printf("Average Fan-Out:      %.2f\n", metrics.AverageFanOut)
	fmt.Printf("Max Fan-Out:          %d\n", metrics.MaxFanOut)
	fmt.Printf("Average Fan-In:       %.2f\n", metrics.AverageFanIn)
	fmt.Printf("Max Fan-In:           %d\n", metrics.MaxFanIn)
	fmt.Printf("Cyclomatic Complexity: %d\n", metrics.CyclomaticComplexity)

	// 5. Export topology as JSON
	fmt.Println("\n📄 JSON EXPORT (Topology):")
	fmt.Println(repeatString("-", 80))
	jsonData, err := json.MarshalIndent(topo, "", "  ")
	if err == nil {
		fmt.Println(string(jsonData))
	}

	// 6. Generate Mermaid flowchart
	fmt.Println("\n📊 MERMAID FLOWCHART:")
	fmt.Println(repeatString("-", 80))
	flowchart := compiled.MermaidFlowchart("TD")

	// Save to .mmd file
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
