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
// ERROR HANDLING:
//
// Executors follow the Go iterator convention with error wrapping:
//
//   - All errors returned in second return value (err)
//   - Node execution failures wrapped with state.ErrNodeExecution
//   - Use errors.Is() to distinguish error types
//
// Example error handling:
//
//	import "errors"
//
//	for result, err := range executor.Run(ctx, compiled, messages) {
//	    if err != nil {
//	        // Check if it's a node execution error
//	        if errors.Is(err, state.ErrNodeExecution) {
//	            // Node failed - may be recoverable
//	            log.Printf("Node execution failed: %v", err)
//	            continue // or implement retry logic
//	        }
//	        // Fatal error - execution stopped
//	        // Examples: context canceled, max iterations, quota exceeded
//	        return fmt.Errorf("execution failed: %w", err)
//	    }
//	    // Process successful result
//	}
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
// // Execute with pregel executor
// executor := exec.NewPregelExecutor()
//
//	for result, err := range executor.Run(ctx, compiled, nil) {
//	    if err != nil {
//	        log.Printf("Fatal error: %v", err)
//	        break
//	    }
//	    // Process result
//	    fmt.Printf("Node %s: %v\n", result.Node, result.Message)
//	}
package exec
