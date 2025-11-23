// Package graph provides the unified graph structure, compilation, and execution for AgentMesh.
//
// This package defines the fundamental building blocks of a computational graph:
//   - Nodes: Executable units that return Commands with state updates and routing
//   - Command: Atomic combination of state updates and routing decisions
//   - Edges: Static directed connections between nodes
//   - Graph: The graph structure and builder
//   - Compiled: Validated, executable graph
//   - Executors: Pluggable execution strategies (Pregel/BSP)
//
// COMMAND PATTERN (RECOMMENDED):
//
// The Command pattern is the unified execution model where nodes return both
// state updates and routing decisions atomically. This provides:
//   - Co-located logic: routing decision right where state is computed
//   - Type safety: routing targets validated at build time
//   - Clear visualization: Mermaid shows all possible routes
//   - Natural control flow: reads like normal Go if/else
//
// BASIC EXAMPLE:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//
//	// Add Command node with dynamic routing
//	builder.AddCommandNode("router",
//	    []string{"option_a", "option_b", graph.EndNode},
//	    func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
//	        decision := analyze(view)
//	        updates := state.Updates{"decision": decision}
//
//	        if decision == "simple" {
//	            return graph.Goto(updates, "option_a"), nil
//	        }
//	        return graph.Goto(updates, "option_b"), nil
//	    },
//	)
//
//	// Add static node (syntactic sugar)
//	builder.AddStaticNode("option_a", []string{graph.EndNode},
//	    func(ctx, view) (state.Updates, error) {
//	        return state.Updates{"result": "done"}, nil
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
//	  - Command: State updates + routing decisions
//	  - Node: Interface with Execute() returning *Command
//	  - Graph: Structure (nodes, edges, builder)
//	  - Compiled: Validated, executable graph
//	  - Executor: Pluggable execution strategy
//	  - Introspection: Topology analysis and Mermaid generation
//
// API METHODS:
//
// Primary methods for building graphs:
//
//	AddCommandNode(name, targets, fn)       - Main method for nodes with routing
//	AddCommandNodeWithRetry(name, ...)      - Command node with retry policy
//	AddStaticNode(name, targets, fn)        - Syntactic sugar for simple nodes
//	AddNode(node)                           - Add pre-constructed node instance
//
// Helper functions for creating Commands:
//
//	Goto(updates, targets...)               - Route to node(s)
//	End(updates...)                         - Terminate execution
//	GotoOne(target, updates...)             - Route to single node
//	Update(updates).Goto(targets...)        - Fluent API
//	Update(updates).End()                   - Fluent API
//
// See examples/ directory for comprehensive examples of Command pattern usage.
package graph
