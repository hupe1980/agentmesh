package graph

import (
	"context"

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
	Execute(ctx context.Context, view *state.ReadView) (*Command, error)

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
func (n *BaseCommandNode) Execute(ctx context.Context, view *state.ReadView) (*Command, error) {
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
