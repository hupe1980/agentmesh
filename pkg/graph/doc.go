// Package graph provides the unified graph structure, compilation, and execution for AgentMesh.
//
// This package defines the fundamental building blocks of a computational graph:
//   - Nodes: Executable units with run functions
//   - Edges: Directed connections between nodes
//   - ConditionalEdges: Dynamic routing based on runtime state
//   - Graph: The graph structure and builder
//   - Compiled: Validated, executable graph
//   - Executors: Pluggable execution strategies (Sequential, Pregel/BSP)
//
// ARCHITECTURE:
//
//	pkg/graph - Unified package containing:
//	  - Graph structure (nodes, edges, builder)
//	  - Compilation (topology, validation)
//	  - Execution (sequential, pregel executors)
//
// EXAMPLE:
//
//	// Create a graph
//	mgr := state.NewManager()
//	g, _ := graph.NewGraph(mgr)
//	g.AddNode(graph.NewBaseNode("start", startNodeFunc))
//	g.AddNode(graph.NewBaseNode("process", processNodeFunc))
//	g.AddEdge(graph.StartNode, "start")
//	g.AddEdge("start", "process")
//	g.AddEdge("process", graph.EndNode)
//
//	// Compile and execute
//	compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())
//	for output, err := range compiled.Run(ctx, input) {
//	    // Handle output
//	}
package graph
