// Package compile provides graph compilation, validation, and topology computation.
//
// This package takes a Graph (from pkg/graph) and compiles it into an
// executable form, validating structure, computing topology, and preparing
// it for execution by pkg/exec.
//
// RESPONSIBILITIES:
//   - Graph validation (structure, edges, conditionals, topology)
//   - Topology computation (incoming/outgoing edges, topological sort)
//   - Conditional edge processing
//   - Creating immutable CompiledGraph for execution
//
// VALIDATION:
//
// The compile package provides comprehensive pre-execution validation to catch
// errors early, before runtime. Validation checks include:
//
//   - Basic structure: nil nodes, empty names, nil RunFunc, reserved names
//   - Edge validation: missing nodes, invalid edge patterns (to START, from END)
//   - Conditional validation: missing nodes, nil condition functions
//   - Topology validation: cycles, unreachable nodes, dead ends
//
// Validation modes:
//
//   - Default: Permissive - allows unreachable nodes, dead ends, cycles
//   - Strict: Enforces all constraints - no cycles, all nodes reachable from START to END
//   - Disabled: Skips all validation (use with caution for performance-critical paths)
//
// ARCHITECTURE:
//
// pkg/graph    - Pure structure (nodes, edges)
// pkg/compile  - Validation & topology (THIS PACKAGE)
// pkg/exec     - Execution strategies
//
// EXAMPLES:
//
// Basic compilation with default validation:
//
//	g, _ := graph.NewGraph(stateManager)
//	g.AddNode(processNode)
//	g.AddEdge(compile.StartNode, "process")
//	g.AddEdge("process", compile.EndNode)
//
//	compiled, err := compile.Compile(g, stateManager)
//	if err != nil {
//	    // Validation failed - error includes all validation issues
//	    log.Fatal(err)
//	}
//
// Strict validation for production:
//
//	compiled, err := compile.Compile(g, stateManager,
//	    compile.WithStrictValidation())
//	if err != nil {
//	    // Strict validation caught: cycles, unreachable nodes, dead ends
//	    log.Fatal(err)
//	}
//
// Custom validation options:
//
//	compiled, err := compile.Compile(g, stateManager,
//	    compile.WithValidation(compile.ValidationOptions{
//	        AllowCycles:      true,  // For iterative algorithms
//	        AllowUnreachable: false, // Reject unreachable nodes
//	        AllowDeadEnds:    false, // Reject dead ends
//	    }))
//
// Disable validation (use with caution):
//
//	compiled, err := compile.Compile(g, stateManager,
//	    compile.WithoutValidation())
//	// No validation performed - graph may have errors that cause runtime failures
//
// Through exec package (recommended):
//
//	runnable, err := exec.CompileGraph(g,
//	    exec.WithStrictValidation())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	results := runnable.Run(ctx, messages)
package compile
