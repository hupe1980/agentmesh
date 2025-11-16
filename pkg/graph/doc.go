// Package graph provides the core graph structure for AgentMesh.
//
// This package defines the fundamental building blocks of a computational graph:
//   - Nodes: Executable units with run functions
//   - Edges: Directed connections between nodes
//   - ConditionalEdges: Dynamic routing based on runtime state
//   - Graph: The graph structure and builder
//
// This package contains NO execution logic - it only defines structure.
// Execution is handled by pkg/exec, and compilation by pkg/compile.
//
// ARCHITECTURE:
//
//	pkg/graph (this)     - Pure graph structure (nodes, edges, builder)
//	pkg/compile          - Graph compilation (topology, validation)
//	pkg/exec             - Graph execution (sequential, pregel strategies)
//
// EXAMPLE:
//
//	builder := graph.NewBuilder()
//	builder.AddNode("start", startNodeFunc)
//	builder.AddNode("process", processNodeFunc)
//	builder.AddEdge("start", "process")
//	g := builder.Build()
//
//	// Compilation and execution happen in other packages
//	compiled := compile.Compile(g)
//	executor := exec.NewSequential()
//	executor.Run(ctx, compiled, ...)
package graph
