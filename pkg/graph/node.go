package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Node is the unified interface for all graph nodes.
// Every node returns a tuple: (targets, updates, error) - simple and idiomatic Go.
//
// All nodes must:
//   - Execute with read-only state access
//   - Return tuple: ([]string targets, state.Updates, error)
//   - Declare all possible routing targets for validation
type Node interface {
	// Name returns the unique identifier for this node in the graph.
	Name() string

	// Execute runs the node logic with read-only state access.
	// Returns (targets, updates, error) tuple:
	//   - targets: where to route next (e.g., []string{"tool"}, []string{END})
	//   - updates: state changes to apply (can be nil if no changes)
	//   - error: any execution error
	Execute(ctx context.Context, view state.ReadView) ([]string, state.Updates, error)

	// Targets returns all possible routing destinations this node can route to.
	// Used for build-time validation and graph visualization.
	// Must include all targets that Execute() might return.
	Targets() []string
}

// NodeWithRetry is an optional interface for nodes that support retry policies.
// Implement this to enable automatic retry behavior on node execution failures.
type NodeWithRetry interface {
	Node
	RetryPolicy() *RetryPolicy
}

// NodeWithNamespace is an optional interface for nodes that operate within a specific namespace.
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
//	node := graph.NewNamespacedNode(
//	    "agent1_process",
//	    agentNS,
//	    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	        // view is filtered - can ONLY access agent1.* keys
//	        status := state.GetFromView(view, statusKey)  // Works
//	        keys := view.Keys()  // Returns only ["status", ...] from agent1
//	        // view.Has("agent2.data") returns false (filtered out)
//	        return []string{graph.END}, updates, nil
//	    },
//	    []string{graph.END},
//	    false, // includeGlobal: don't expose global keys
//	)
type NodeWithNamespace interface {
	Node
	Namespace() state.Namespace
}

// NodeFunc is THE function signature for all node logic.
// Returns a tuple: (targets, updates, error)
//
// The function receives:
//   - ctx: Context for cancellation and request-scoped values
//   - view: Read-only view of the current state
//
// It returns:
//   - []string: Target nodes to route to (e.g., []string{"tool"}, []string{END})
//   - state.Updates: State changes to apply (can be nil if no changes)
//   - error: Any execution error (nil on success)
//
// Example:
//
//	func routerNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	    msgs := state.GetFromView(view, messagesKey)
//	    if needsTool {
//	        return []string{"tool"}, nil, nil
//	    }
//	    return []string{graph.END}, state.Updates{
//	        messagesKey.Name(): append(msgs, response),
//	    }, nil
//	}
type NodeFunc func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error)

// BaseNode is THE standard Node implementation.
// All nodes use this - wraps NodeFunc with target declaration.
//
// Use this to create reusable nodes:
//
//	node := &graph.BaseNode{
//	    NodeName: "router",
//	    Fn: func(ctx, view) ([]string, state.Updates, error) {
//	        if condition {
//	            return []string{"tool"}, updates, nil
//	        }
//	        return []string{graph.END}, updates, nil
//	    },
//	    DeclaredTargets: []string{"tool", graph.END},
//	    Retry: graph.NewRetryPolicy().WithMaxAttempts(5).Build(), // Optional
//	}
//	builder.AddNode(node)
type BaseNode struct {
	NodeName        string
	Fn              NodeFunc
	DeclaredTargets []string
	Retry           *RetryPolicy // Optional: enables automatic retry on errors
}

// Name returns the node's name.
func (n *BaseNode) Name() string {
	return n.NodeName
}

// Execute runs the node's NodeFunc.
func (n *BaseNode) Execute(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
	if n.Fn == nil {
		return []string{EndNode}, nil, nil
	}
	return n.Fn(ctx, view)
}

// Targets returns the declared routing targets.
func (n *BaseNode) Targets() []string {
	return n.DeclaredTargets
}

// RetryPolicy returns the node's retry policy if set.
func (n *BaseNode) RetryPolicy() *RetryPolicy {
	return n.Retry
}

// NamespacedNode is a Node implementation that operates within a specific namespace.
// During execution, the node receives a NamespacedReadView that ONLY exposes keys from its declared namespace.
// Optionally, global (non-namespaced) keys can also be exposed via the includeGlobal parameter.
// This provides actual runtime enforcement of state isolation with validation of returned updates.
//
// Use this to create nodes with guaranteed isolated state:
//
//	agentNS := state.MustNamespace("agent1")
//	node := graph.NewNamespacedNode(
//	    "agent1_process",
//	    agentNS,
//	    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	        // This view is filtered - only contains "agent1.*" keys
//	        // Attempting to access other namespace keys will fail
//	        // Returned updates are validated to only contain agent1.* keys
//	        status := state.GetFromView(view, statusKey)
//	        // view.Keys() returns only ["status", ...] from agent1 namespace
//	        return []string{graph.END}, updates, nil
//	    },
//	    []string{graph.END},
//	    false, // includeGlobal
//	)
type NamespacedNode struct {
	BaseNode
	namespace     state.Namespace
	includeGlobal bool // If true, node can also access and update global keys
}

// NewNamespacedNode creates a new namespaced node.
// The node receives a filtered NamespacedReadView during execution that only exposes keys from its namespace.
// Returned updates are validated to ensure they only contain keys from the allowed namespaces.
//
// Parameters:
//   - name: unique node identifier
//   - ns: namespace to scope state access to
//   - fn: node function that receives filtered view and returns (targets, updates, error)
//   - targets: declared routing targets
//   - includeGlobal: if true, global (non-namespaced) keys are also visible and can be updated
//
// Example:
//
//	validationNS := state.MustNamespace("validation")
//	node := graph.NewNamespacedNode(
//	    "validate",
//	    validationNS,
//	    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	        // view is filtered - only validation.* keys are visible
//	        // view.Keys() returns only ["data", "valid", ...] without namespace prefix
//	        data := state.GetFromView(view, validationDataKey)
//	        if !isValid(data) {
//	            return []string{graph.END}, updates, nil
//	        }
//	        return []string{"enrich"}, updates, nil
//	    },
//	    []string{"enrich", graph.END},
//	    false, // includeGlobal: only validation.* keys
//	)
func NewNamespacedNode(
	name string,
	ns state.Namespace,
	fn NodeFunc,
	targets []string,
	includeGlobal bool,
) *NamespacedNode {
	return &NamespacedNode{
		BaseNode: BaseNode{
			NodeName:        name,
			Fn:              fn,
			DeclaredTargets: targets,
		},
		namespace:     ns,
		includeGlobal: includeGlobal,
	}
}

// NewNamespacedNodeWithRetry creates a namespaced node with retry policy.
func NewNamespacedNodeWithRetry(
	name string,
	ns state.Namespace,
	fn NodeFunc,
	targets []string,
	retry *RetryPolicy,
	includeGlobal bool,
) *NamespacedNode {
	return &NamespacedNode{
		BaseNode: BaseNode{
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
func (n *NamespacedNode) Namespace() state.Namespace {
	return n.namespace
}

// Execute runs the node's NodeFunc with a namespace-filtered view.
// The NodeFunc receives a NamespacedReadView that only exposes keys from this node's namespace.
// If includeGlobal is true, global (non-namespaced) keys are also visible.
// Validates that returned updates only contain keys from the node's allowed namespaces.
func (n *NamespacedNode) Execute(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
	if n.Fn == nil {
		return []string{EndNode}, nil, nil
	}

	// Create a filtered view that only exposes this node's namespace (and global if allowed)
	namespacedView := state.NewNamespacedReadView(view, n.namespace, n.includeGlobal)

	targets, updates, err := n.Fn(ctx, namespacedView)
	if err != nil {
		return nil, nil, err
	}

	// Validate that updates only contain keys from allowed namespaces
	if updates != nil {
		if err := n.validateUpdates(updates); err != nil {
			return nil, nil, err
		}
	}

	return targets, updates, nil
}

// validateUpdates checks that all update keys belong to the node's namespace or global (if allowed).
func (n *NamespacedNode) validateUpdates(updates state.Updates) error {
	for key := range updates {
		if !n.isAllowedKey(key) {
			if n.includeGlobal {
				return fmt.Errorf("%w: node %s in namespace %s attempted to update key %s (only %s and global keys are allowed)",
					ErrNamespaceViolation, n.NodeName, n.namespace.Name(), key, n.namespace.Name())
			}
			return fmt.Errorf("%w: node %s in namespace %s attempted to update key %s (only %s keys are allowed)",
				ErrNamespaceViolation, n.NodeName, n.namespace.Name(), key, n.namespace.Name())
		}
	}
	return nil
}

// isAllowedKey checks if a key is allowed for this node (belongs to node's namespace or is global if allowed).
func (n *NamespacedNode) isAllowedKey(key string) bool {
	// Check if key belongs to node's namespace
	if state.IsNamespaced(key) {
		ns, _ := state.ParseNamespacedKey(key)
		return ns == n.namespace.Name()
	}
	// Key is global (no namespace prefix)
	return n.includeGlobal
}
