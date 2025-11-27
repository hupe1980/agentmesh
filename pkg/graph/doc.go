// Package graph provides the unified graph structure, compilation, and execution for AgentMesh.
//
// This package defines the fundamental building blocks of a computational graph:
//   - Nodes: Executable units that return tuples (targets, updates, error)
//   - Edges: Static directed connections between nodes
//   - Graph: The graph structure and builder
//   - Compiled: Validated, executable graph
//   - Executors: Pluggable execution strategies (Pregel/BSP)
//   - Command: Fluent builder for constructing state updates (optional)
//
// TUPLE-BASED API:
//
// Nodes return a simple tuple: ([]string, state.Updates, error)
//   - []string: Target node names for routing
//   - state.Updates: Map of state updates
//   - error: Any errors encountered
//
// This provides:
//   - Co-located logic: routing decision right where state is computed
//   - Type safety: routing targets validated at build time
//   - Clear visualization: Mermaid shows all possible routes
//   - Natural control flow: reads like normal Go if/else
//
// BASIC EXAMPLE:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//
//	var DecisionKey = state.NewKey[string]("decision", "")
//	var ResultKey = state.NewKey[string]("result", "")
//
//	// Add node with dynamic routing using map literal
//	builder.AddNodeFunc("router",
//	    []string{"option_a", "option_b", graph.EndNode},
//	    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	        decision := analyze(view)
//	        updates := state.Updates{DecisionKey.Name(): decision}
//
//	        if decision == "simple" {
//	            return []string{"option_a"}, updates, nil
//	        }
//	        return []string{"option_b"}, updates, nil
//	    },
//	)
//
//	// Or use Command builder for cleaner syntax
//	builder.AddNodeFunc("router",
//	    []string{"option_a", "option_b", graph.EndNode},
//	    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	        decision := analyze(view)
//	        if decision == "simple" {
//	            return command.New().
//	                Set(DecisionKey, decision).
//	                To("option_a")
//	        }
//	        return command.New().
//	            Set(DecisionKey, decision).
//	            To("option_b")
//	    },
//	)
//
//	// Compile and execute
//	compiled, _ := builder.Compile()
//	for output, err := range compiled.Run(ctx, input) {
//	    // Handle output
//	}
//
// ARCHITECTURE:
//
//	pkg/graph - Unified package containing:
//	  - Node: Interface with Execute() returning tuple
//	  - Graph: Structure (nodes, edges, builder)
//	  - Compiled: Validated, executable graph
//	  - Executor: Pluggable execution strategy
//	  - Command: Optional fluent builder for state updates
//	  - Introspection: Topology analysis and Mermaid generation
//
// API METHODS:
//
// Primary methods for building graphs:
//
//	AddNodeFunc(name, targets, fn)          - Main method for adding nodes
//	AddNodeFuncWithRetry(name, ...)         - Node with retry policy
//	AddNode(node)                           - Add pre-constructed node instance
//
// Helper for creating state updates:
//
//	NewCommand()                            - Create Command builder
//	  .Set(key, value)                      - Add state update
//	  .Build()                              - Get (updates, error)
//	  .To(targets...)                       - Get complete tuple
//
// See examples/ directory for comprehensive examples.
package graph
