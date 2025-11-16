// Package compile provides graph compilation and topology computation.
//
// This package takes a Graph (from pkg/graph) and compiles it into an
// executable form, computing topology, validating structure, and preparing
// it for execution by pkg/exec.
//
// RESPONSIBILITIES:
//   - Topology computation (incoming/outgoing edges, topological sort)
//   - Graph validation (cycles, missing nodes, etc.)
//   - Conditional edge processing
//   - Creating immutable CompiledGraph for execution
//
// ARCHITECTURE:
//
// pkg/graph    - Pure structure (nodes, edges)
// pkg/compile  - Compilation & topology (THIS PACKAGE)
// pkg/exec     - Execution strategies
//
// EXAMPLE:
//
// g := graph.NewBuilder().
// Applications/AddNode("start", startFunc).
// Applications/AddNode("end", endFunc).
// Applications/AddEdge("start", "end").
// Applications/Build()
//
// compiled, err := compile.Compile(g, stateManager)
// if err != nil {
// Applications// Handle compilation errors
// }
//
// // Now pass to executor
// executor := exec.NewSequential()
// results := executor.Run(ctx, compiled, ...)
package compile
