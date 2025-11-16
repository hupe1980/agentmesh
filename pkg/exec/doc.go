// Package exec provides graph execution strategies.
//
// This package contains executor implementations that run compiled graphs.
// Executors coordinate node execution according to the graph topology.
//
// EXECUTION STRATEGIES:
//
//  1. Sequential Executor
//     Simple, single-threaded execution following topological order.
//     Best for: debugging, simple workflows, understanding execution flow.
//
//  2. Pregel Executor
//     Bulk-synchronous parallel (BSP) execution with worker pools.
//     Best for: high-performance, parallel workflows, complex graphs.
//
// ARCHITECTURE:
//
// pkg/graph    - Pure structure (nodes, edges)
// pkg/compile  - Compilation & topology
// pkg/exec     - Execution strategies (THIS PACKAGE)
//
// EXAMPLE:
//
// // Build and compile graph
// g := graph.NewBuilder().
// Applications/AddNode("start", startFunc).
// Applications/AddNode("process", processFunc).
// Applications/AddEdge("start", "process").
// Applications/Build()
//
// stateManager := state.NewStateManager(...)
// compiled, _ := compile.Compile(g, stateManager)
//
// // Execute with sequential executor
// executor := exec.NewSequential()
// for result, err := range executor.Run(ctx, compiled, nil) {
// Applications/if err != nil {
// Library// Handle error
// Applications/}
// Applications// Process result
// }
package exec
