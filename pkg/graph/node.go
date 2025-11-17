package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Node represents a vertex in the execution graph.
type Node struct {
	Name        string
	RunFunc     func(ctx context.Context, view *state.ReadView) (*NodeResult, error)
	RetryPolicy *RetryPolicy // Optional retry configuration for this node
}

// NodeResult contains the output of a node execution.
// Nodes return generic state updates - the agent layer decides what those updates represent.
type NodeResult struct {
	Updates state.Updates // State updates (type-safe, key-value pairs)
}

// Run executes the node's function.
func (n *Node) Run(ctx context.Context, view *state.ReadView) (*NodeResult, error) {
	if n.RunFunc == nil {
		return nil, nil
	}
	return n.RunFunc(ctx, view)
}
