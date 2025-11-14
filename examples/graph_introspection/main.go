package main

import (
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
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

func main() {
	// Build a complex graph with conditional routing
	builder, err := graph.NewBuilder()
	if err != nil {
		panic(err)
	}

	// Add nodes
	builder.Node("input_validator", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		fmt.Println("✓ Validating input...")
		return &graph.NodeResult{
			Updates: map[string]any{
				"valid":    true,
				"priority": "high",
			},
		}, nil
	})

	builder.Node("router", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		fmt.Println("✓ Routing request...")
		return &graph.NodeResult{}, nil
	})

	builder.Node("high_priority_handler", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		fmt.Println("✓ Handling high priority request...")
		return &graph.NodeResult{
			Updates: map[string]any{"processed": true},
		}, nil
	})

	builder.Node("normal_handler", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		fmt.Println("✓ Handling normal request...")
		return &graph.NodeResult{
			Updates: map[string]any{"processed": true},
		}, nil
	})

	builder.Node("aggregator", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		fmt.Println("✓ Aggregating results...")
		return &graph.NodeResult{
			Updates: map[string]any{"complete": true},
		}, nil
	})

	// Define edges
	builder.AddEdge(graph.StartNode, "input_validator")
	builder.AddEdge("input_validator", "router")

	// Conditional routing based on priority
	builder.AddConditionalEdges("router", func(ctx context.Context, s graphstate.Reader) []string {
		priority := s.Get("priority")
		if priority == "high" {
			return []string{"high_priority_handler"}
		}
		return []string{"normal_handler"}
	}, []string{"high_priority_handler", "normal_handler"})

	builder.AddEdge("high_priority_handler", "aggregator")
	builder.AddEdge("normal_handler", "aggregator")
	builder.AddEdge("aggregator", graph.EndNode)

	// Compile the graph
	compiled, err := builder.Compile()
	if err != nil {
		log.Fatal(err)
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
		fmt.Printf("  Is Conditional:     %v\n", info.IsConditional)
		fmt.Printf("  Is Conditional Gate: %v\n", info.IsConditionalGate)
		fmt.Printf("  Has Retry Policy:   %v\n", info.HasRetryPolicy)
	}

	// 3. Get topology overview
	fmt.Println("\n🗺️  TOPOLOGY OVERVIEW:")
	fmt.Println(repeatString("-", 80))
	topo := compiled.GetTopology()
	fmt.Printf("Entry Points:      %v\n", topo.EntryPoints)
	fmt.Printf("Exit Points:       %v\n", topo.ExitPoints)
	fmt.Printf("Conditional Nodes: %v\n", topo.ConditionalNodes)
	fmt.Printf("Max Depth:         %d\n", topo.MaxDepth)
	fmt.Printf("Estimated Paths:   %d\n", topo.TotalPaths)

	// 4. Get metrics
	fmt.Println("\n📊 GRAPH METRICS:")
	fmt.Println(repeatString("-", 80))
	metrics := compiled.GetMetrics()
	fmt.Printf("Total Nodes:          %d\n", metrics.TotalNodes)
	fmt.Printf("Total Edges:          %d\n", metrics.TotalEdges)
	fmt.Printf("Conditional Edges:    %d\n", metrics.ConditionalEdges)
	fmt.Printf("Average Fan-Out:      %.2f\n", metrics.AverageFanOut)
	fmt.Printf("Max Fan-Out:          %d\n", metrics.MaxFanOut)
	fmt.Printf("Average Fan-In:       %.2f\n", metrics.AverageFanIn)
	fmt.Printf("Max Fan-In:           %d\n", metrics.MaxFanIn)
	fmt.Printf("Cyclomatic Complexity: %d\n", metrics.CyclomaticComplexity)

	// 5. Get dependencies for a specific node
	fmt.Println("\n🔗 DEPENDENCIES (router node):")
	fmt.Println(repeatString("-", 80))
	deps, err := compiled.GetDependencies("router")
	if err == nil {
		fmt.Printf("Direct Predecessors:  %v\n", deps.DirectPredecessors)
		fmt.Printf("Direct Successors:    %v\n", deps.DirectSuccessors)
		fmt.Printf("All Predecessors:     %v\n", deps.AllPredecessors)
		fmt.Printf("All Successors:       %v\n", deps.AllSuccessors)
		fmt.Printf("Depth from START:     %d\n", deps.Depth)
	}

	// 6. Get all possible execution paths
	fmt.Println("\n🛤️  EXECUTION PATHS:")
	fmt.Println(repeatString("-", 80))
	paths := compiled.GetExecutionPath(10)
	for i, path := range paths {
		fmt.Printf("Path %d: %v\n", i+1, path)
	}

	// 7. Export topology as JSON for external visualization
	fmt.Println("\n📄 JSON EXPORT (Topology):")
	fmt.Println(repeatString("-", 80))
	jsonData, err := json.MarshalIndent(topo, "", "  ")
	if err == nil {
		fmt.Println(string(jsonData))
	}

	// 8. Execute the graph and show runtime metrics
	fmt.Println("\n" + repeatString("=", 80))
	fmt.Println("EXECUTING GRAPH")
	fmt.Println(repeatString("=", 80) + "\n")

	_, err = graph.Last(compiled.Run(context.Background(), nil))
	if err != nil {
		log.Printf("Execution error: %v", err)
	}

	// Get runtime metrics after execution
	fmt.Println("\n📈 RUNTIME METRICS (After Execution):")
	fmt.Println(repeatString("-", 80))
	runtimeMetrics := compiled.GetMetrics()
	fmt.Printf("Current Superstep: %d\n", runtimeMetrics.CurrentSuperstep)
	fmt.Printf("Completed Nodes:   %v\n", runtimeMetrics.CompletedNodes)
	fmt.Printf("Paused Nodes:      %v\n", runtimeMetrics.PausedNodes)

	// 9. Generate Mermaid flowchart and save to file
	fmt.Println("\n📊 GENERATE MERMAID FLOWCHART:")
	fmt.Println(repeatString("-", 80))

	// Generate flowchart using introspection API
	flowchart := compiled.GenerateMermaidFlowchart("TD")

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
