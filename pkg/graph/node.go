package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Node is the unified interface for all graph nodes.
// Every node returns Command - ONE execution model.
//
// All nodes must:
//   - Execute with read-only state access
//   - Return Command with state updates and routing decision
//   - Declare all possible routing targets for validation
type Node interface {
	// Name returns the unique identifier for this node in the graph.
	Name() string

	// Execute runs the node logic with read-only state access.
	// Returns Command with state updates and routing decision.
	Execute(ctx context.Context, view state.ReadView) (*Command, error)

	// Targets returns all possible routing destinations this node can route to.
	// Used for build-time validation and graph visualization.
	// Must include all targets that Execute() might return in Command.Goto.
	Targets() []string
}

// NodeWithRetry is an optional interface for nodes that support retry policies.
// Implement this to enable automatic retry behavior on node execution failures.
type NodeWithRetry interface {
	Node
	RetryPolicy() *RetryPolicy
}

// NamespacedNode is an optional interface for nodes that operate within a specific namespace.
// Nodes implementing this interface receive a filtered ReadView during execution that ONLY exposes
// keys from their declared namespace. This provides runtime enforcement of state isolation.
//
// State isolation is enforced at runtime by:
//  1. The Execute() method receives a NamespacedReadView (not full ReadView)
//  2. NamespacedReadView.Keys() only returns keys from the node's namespace
//  3. NamespacedReadView.Has() only returns true for keys in the node's namespace
//  4. Attempting to access keys from other namespaces will fail
//
// This enables:
//   - Multi-agent systems with guaranteed state isolation per agent
//   - Pipeline stages that cannot access each other's state
//   - Runtime enforcement of state boundaries (not just convention)
//   - Graph introspection to understand state dependencies
//
// Example:
//
//	agentNS := state.MustNamespace("agent1")
//	statusKey := state.TypedKey[string](agentNS, "status", "idle")  // Creates "agent1.status"
//
//	node := graph.NewNamespacedCommandNode(
//	    "agent1_process",
//	    agentNS,
//	    func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
//	        // view is filtered - can ONLY access agent1.* keys
//	        status := state.GetFromView(view, statusKey)  // Works
//	        keys := view.Keys()  // Returns only ["status", ...] from agent1
//	        // view.Has("agent2.data") returns false (filtered out)
//	    },
//	    targets,
//	    false, // includeGlobal: don't expose global keys
//	)
type NamespacedNode interface {
	Node
	Namespace() state.Namespace
}

// BaseCommandNode is THE standard Node implementation.
// All nodes use this - wraps CommandFunc with target declaration.
//
// Use this to create reusable nodes that can be instantiated multiple times:
//
//	targets := graph.NewTargetSet("target1", graph.EndNode)
//	node := &graph.BaseCommandNode{
//	    NodeName: "router",
//	    Fn: func(ctx, view) (*graph.Command, error) {
//	        if condition {
//	            return targets.Goto(targets.Get("target1"), updates), nil
//	        }
//	        return targets.Goto(targets.Get(graph.EndNode), updates), nil
//	    },
//	    DeclaredTargets: targets,
//	    RetryPolicy: graph.NewRetryPolicy().WithMaxAttempts(5).Build(), // Optional
//	}
//	builder.AddNode(node)
type BaseCommandNode struct {
	NodeName        string
	Fn              CommandFunc
	DeclaredTargets *TargetSet
	Retry           *RetryPolicy // Optional: enables automatic retry on errors
}

// Name returns the node's name.
func (n *BaseCommandNode) Name() string {
	return n.NodeName
}

// Execute runs the node's CommandFunc.
func (n *BaseCommandNode) Execute(ctx context.Context, view state.ReadView) (*Command, error) {
	if n.Fn == nil {
		return End(), nil
	}
	return n.Fn(ctx, view)
}

// Targets returns the declared routing targets as a slice.
func (n *BaseCommandNode) Targets() []string {
	if n.DeclaredTargets == nil {
		return nil
	}
	return n.DeclaredTargets.All()
}

// TargetSet returns the node's TargetSet.
func (n *BaseCommandNode) TargetSet() *TargetSet {
	return n.DeclaredTargets
}

// RetryPolicy returns the node's retry policy if set.
func (n *BaseCommandNode) RetryPolicy() *RetryPolicy {
	return n.Retry
}

// NamespacedCommandNode is a Node implementation that operates within a specific namespace.
// During execution, the node receives a NamespacedReadView that ONLY exposes keys from its declared namespace.
// Optionally, global (non-namespaced) keys can also be exposed via the includeGlobal parameter.
// This provides actual runtime enforcement of state isolation with validation of returned updates.
//
// Use this to create nodes with guaranteed isolated state:
//
//	agentNS := state.MustNamespace("agent1")
//	node := graph.NewNamespacedCommandNode(
//	    "agent1_process",
//	    agentNS,
//	    func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
//	        // This view is filtered - only contains "agent1.*" keys
//	        // Attempting to access other namespace keys will fail
//	        // Returned updates are validated to only contain agent1.* keys
//	        status := state.GetFromView(view, statusKey)
//	        // view.Keys() only returns ["status", ...] from agent1 namespace
//	        return graph.End(), nil
//	    },
//	    targets,
//	    false, // includeGlobal
//	)
type NamespacedCommandNode struct {
	BaseCommandNode
	namespace     state.Namespace
	includeGlobal bool // If true, node can also access and update global keys
}

// NewNamespacedCommandNode creates a new namespaced command node.
// The node receives a filtered NamespacedReadView during execution that only exposes keys from its namespace.
// Returned updates are validated to ensure they only contain keys from the allowed namespaces.
//
// Parameters:
//   - name: unique node identifier
//   - ns: namespace to scope state access to
//   - fn: command function that receives filtered view
//   - targets: declared routing targets
//   - includeGlobal: if true, global (non-namespaced) keys are also visible and can be updated
//
// Example:
//
//	validationNS := state.MustNamespace("validation")
//	targets := graph.NewTargetSet("enrich", graph.EndNode)
//	node := graph.NewNamespacedCommandNode(
//	    "validate",
//	    validationNS,
//	    func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
//	        // view is filtered - only validation.* keys are visible
//	        // view.Keys() returns only ["data", "valid", ...] without namespace prefix
//	        data := state.GetFromView(view, validationDataKey)
//	        if !isValid(data) {
//	            return targets.End(updates), nil
//	        }
//	        return targets.Goto(targets.Get("enrich"), updates), nil
//	    },
//	    targets,
//	    false, // includeGlobal: only validation.* keys
//	)
func NewNamespacedCommandNode(
	name string,
	ns state.Namespace,
	fn CommandFunc,
	targets *TargetSet,
	includeGlobal bool,
) *NamespacedCommandNode {
	return &NamespacedCommandNode{
		BaseCommandNode: BaseCommandNode{
			NodeName:        name,
			Fn:              fn,
			DeclaredTargets: targets,
		},
		namespace:     ns,
		includeGlobal: includeGlobal,
	}
}

// NewNamespacedCommandNodeWithRetry creates a namespaced node with retry policy.
func NewNamespacedCommandNodeWithRetry(
	name string,
	ns state.Namespace,
	fn CommandFunc,
	targets *TargetSet,
	retry *RetryPolicy,
	includeGlobal bool,
) *NamespacedCommandNode {
	return &NamespacedCommandNode{
		BaseCommandNode: BaseCommandNode{
			NodeName:        name,
			Fn:              fn,
			DeclaredTargets: targets,
			Retry:           retry,
		},
		namespace:     ns,
		includeGlobal: includeGlobal,
	}
}

// Namespace returns the namespace this node is scoped to.
func (n *NamespacedCommandNode) Namespace() state.Namespace {
	return n.namespace
}

// Execute runs the node's CommandFunc with a namespace-filtered view.
// The CommandFunc receives a NamespacedReadView that only exposes keys from this node's namespace.
// If includeGlobal is true, global (non-namespaced) keys are also visible.
// Validates that returned updates only contain keys from the node's allowed namespaces.
func (n *NamespacedCommandNode) Execute(ctx context.Context, view state.ReadView) (*Command, error) {
	if n.Fn == nil {
		return End(), nil
	}

	// Create a filtered view that only exposes this node's namespace (and global if allowed)
	namespacedView := state.NewNamespacedReadView(view, n.namespace, n.includeGlobal)

	cmd, err := n.Fn(ctx, namespacedView)
	if err != nil {
		return nil, err
	}

	// Validate that updates only contain keys from allowed namespaces
	if cmd != nil && cmd.Updates != nil {
		if err := n.validateUpdates(cmd.Updates); err != nil {
			return nil, err
		}
	}

	return cmd, nil
}

// validateUpdates checks that all update keys belong to the node's namespace or global (if allowed).
func (n *NamespacedCommandNode) validateUpdates(updates state.Updates) error {
	for key := range updates {
		if !n.isAllowedKey(key) {
			if n.includeGlobal {
				return fmt.Errorf("node %q in namespace %q attempted to update key %q which belongs to a different namespace (only %q and global keys are allowed)",
					n.NodeName, n.namespace.Name(), key, n.namespace.Name())
			}
			return fmt.Errorf("node %q in namespace %q attempted to update key %q which belongs to a different namespace (only %q keys are allowed)",
				n.NodeName, n.namespace.Name(), key, n.namespace.Name())
		}
	}
	return nil
}

// isAllowedKey checks if a key is allowed for this node (belongs to node's namespace or is global if allowed).
func (n *NamespacedCommandNode) isAllowedKey(key string) bool {
	// Check if key belongs to node's namespace
	if state.IsNamespaced(key) {
		ns, _ := state.ParseNamespacedKey(key)
		return ns == n.namespace.Name()
	}
	// Key is global (no namespace prefix)
	return n.includeGlobal
}
