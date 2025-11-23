package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Command represents a node's execution result with routing decision.
//
// Command pattern: nodes return both state updates
// and routing decisions atomically. This provides a unified execution model
// where every node explicitly declares where to go next.
//
// The Command pattern eliminates the separation between computation and routing,
// making control flow explicit and co-located with the logic that produces it.
type Command struct {
	// Updates are the state changes to apply after this node executes.
	// Can be nil if the node produces no state changes.
	Updates state.Updates

	// Goto specifies the routing decision - where to execute next.
	// - Single node: []string{"next_node"}
	// - Multiple nodes (parallel): []string{"node1", "node2"}
	// - End execution: []string{EndNode}
	// - Empty/nil is invalid and will cause execution error
	Goto []string
}

// CommandFunc is THE function signature for all node logic.
// Every node function uses this signature - returns Command with routing.
//
// The function receives:
//   - ctx: Context for cancellation and request-scoped values
//   - view: Read-only view of the current state
//
// It must return:
//   - *Command: State updates and routing decision
//   - error: Any execution error (nil on success)
type CommandFunc func(ctx context.Context, view *state.ReadView) (*Command, error)

// Goto creates a Command that routes to a single target with optional updates.
//
// Example:
//
//	return graph.Goto("tool_node", state.Updates{"messages": append(msgs, response)})
func Goto(target string, updates ...state.Updates) *Command {
	var merged state.Updates
	if len(updates) > 0 {
		merged = updates[0]
	}
	return &Command{
		Updates: merged,
		Goto:    []string{target},
	}
}

// GotoOne creates a Command that routes to a single node with optional updates.
// Convenience function for the common single-target case.
//
// Example:
//
//	return graph.GotoOne("next", state.Updates{"count": count + 1})
func GotoOne(target string, updates ...state.Updates) *Command {
	return Goto(target, updates...)
}

// GotoAll creates a Command that routes to multiple targets (parallel execution).
//
// Example:
//
//	return graph.GotoAll([]string{"task1", "task2"}, state.Updates{"started": true})
func GotoAll(targets []string, updates ...state.Updates) *Command {
	var merged state.Updates
	if len(updates) > 0 {
		merged = updates[0]
	}
	return &Command{
		Updates: merged,
		Goto:    targets,
	}
}

// End creates a Command that ends execution with optional updates.
//
// Example:
//
//	return graph.End(state.Updates{"final_result": result})
func End(updates ...state.Updates) *Command {
	var merged state.Updates
	if len(updates) > 0 {
		merged = updates[0]
	}
	return &Command{
		Updates: merged,
		Goto:    []string{EndNode},
	}
}

// TargetSet provides compile-time type safety for routing targets.
// Create a TargetSet to ensure all routing decisions use valid, declared targets.
// Targets are maintained in declaration order for ordered helpers (GotoAll, GotoFirst, GotoLast).
//
// Example:
//
//	targets := graph.NewTargetSet("tool", "model", graph.EndNode)
//	builder.AddCommandNodeTyped("router", targets,
//	    func(ctx, view) (*graph.Command, error) {
//	        if needsTool {
//	            return targets.Goto(targets.Get("tool"), updates), nil
//	        }
//	        // Route to EndNode explicitly
//	        return targets.Goto(targets.Get(graph.EndNode), updates), nil
//	    })
type TargetSet struct {
	targets map[string]string
	all     []string // Maintained in declaration order
}

// NewTargetSet creates a new type-safe target set from the given target names.
// Targets are stored in the order they are declared, which is used by ordered
// helpers like GotoAll, GotoFirst, and GotoLast.
//
// Example:
//
//	targets := graph.NewTargetSet("node_a", "node_b", graph.EndNode)
func NewTargetSet(targets ...string) *TargetSet {
	ts := &TargetSet{
		targets: make(map[string]string, len(targets)),
		all:     make([]string, 0, len(targets)),
	}
	for _, target := range targets {
		ts.targets[target] = target
		ts.all = append(ts.all, target)
	}
	return ts
}

// Get returns the target name if it exists in the set, or empty string if not.
// Use this to get type-safe target references.
//
// Example:
//
//	target := targets.Get("tool")  // Returns "tool" or ""
func (ts *TargetSet) Get(target string) string {
	return ts.targets[target]
}

// Has checks if a target exists in the set.
func (ts *TargetSet) Has(target string) bool {
	_, ok := ts.targets[target]
	return ok
}

// All returns all targets in the set as a slice.
// Use this when calling AddCommandNodeTyped.
func (ts *TargetSet) All() []string {
	return ts.all
}

// Goto creates a Command routing to a single target with optional updates.
//
// Example:
//
//	return targets.Goto(targets.Get("next"), updates)
func (ts *TargetSet) Goto(target string, updates ...state.Updates) *Command {
	var merged state.Updates
	if len(updates) > 0 {
		merged = updates[0]
	}
	return &Command{
		Updates: merged,
		Goto:    []string{target},
	}
}

// GotoOne creates a Command routing to a single target.
//
// Example:
//
//	return targets.GotoOne(targets.Get("next"), updates)
func (ts *TargetSet) GotoOne(target string, updates ...state.Updates) *Command {
	var merged state.Updates
	if len(updates) > 0 {
		merged = updates[0]
	}
	return &Command{
		Updates: merged,
		Goto:    []string{target},
	}
}

// GotoAll creates a Command that routes to all targets in the set (parallel execution).
// Targets are routed in declaration order.
//
// Example:
//
//	// Execute all branches in parallel
//	targets := graph.NewTargetSet("branch_a", "branch_b", "branch_c")
//	return targets.GotoAll(updates)
func (ts *TargetSet) GotoAll(updates state.Updates) *Command {
	return &Command{
		Updates: updates,
		Goto:    ts.all,
	}
}

// GotoFirst creates a Command that routes to the first target in the set.
// Useful when you have an ordered set of fallback targets.
//
// Example:
//
//	// Route to primary target
//	targets := graph.NewTargetSet("primary", "secondary", "fallback")
//	return targets.GotoFirst(updates)
func (ts *TargetSet) GotoFirst(updates state.Updates) *Command {
	if len(ts.all) == 0 {
		panic("TargetSet.GotoFirst() called on empty TargetSet")
	}
	return &Command{
		Updates: updates,
		Goto:    []string{ts.all[0]},
	}
}

// GotoLast creates a Command that routes to the last target in the set.
// Useful for ordered workflows where the last target is a common destination.
//
// Example:
//
//	// Route to final aggregation node
//	targets := graph.NewTargetSet("process_a", "process_b", "aggregate")
//	return targets.GotoLast(updates)
func (ts *TargetSet) GotoLast(updates state.Updates) *Command {
	if len(ts.all) == 0 {
		panic("TargetSet.GotoLast() called on empty TargetSet")
	}
	return &Command{
		Updates: updates,
		Goto:    []string{ts.all[len(ts.all)-1]},
	}
}
