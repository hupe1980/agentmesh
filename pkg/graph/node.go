package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Node is the interface that all node types must implement.
// Nodes are the vertices in the execution graph that perform computations.
type Node interface {
	// Name returns the unique identifier for this node in the graph.
	Name() string

	// Execute runs the node logic with read-only state access.
	// Returns state updates to be applied after the BSP barrier.
	Execute(ctx context.Context, view *state.ReadView) (state.Updates, error)
}

// NodeWithRetry is an optional interface for nodes that support retry policies.
type NodeWithRetry interface {
	Node
	RetryPolicy() *RetryPolicy
}

// BaseNode provides a simple implementation of Node using a function.
// This is useful for quick prototyping or simple nodes that don't need
// custom types.
type BaseNode struct {
	name        string
	executeFunc func(ctx context.Context, view *state.ReadView) (state.Updates, error)
	retryPolicy *RetryPolicy
}

// NewBaseNode creates a node from a function.
func NewBaseNode(name string, fn func(ctx context.Context, view *state.ReadView) (state.Updates, error)) *BaseNode {
	return &BaseNode{
		name:        name,
		executeFunc: fn,
	}
}

// NewBaseNodeWithRetry creates a node from a function with retry policy.
func NewBaseNodeWithRetry(name string, fn func(ctx context.Context, view *state.ReadView) (state.Updates, error), policy *RetryPolicy) *BaseNode {
	return &BaseNode{
		name:        name,
		executeFunc: fn,
		retryPolicy: policy,
	}
}

// Name returns the node's name.
func (n *BaseNode) Name() string {
	return n.name
}

// Execute runs the node's function.
func (n *BaseNode) Execute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
	if n.executeFunc == nil {
		return state.Updates{}, nil
	}
	return n.executeFunc(ctx, view)
}

// RetryPolicy returns the node's retry policy if set.
func (n *BaseNode) RetryPolicy() *RetryPolicy {
	return n.retryPolicy
}
