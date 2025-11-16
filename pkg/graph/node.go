package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Node represents a vertex in the execution graph.
type Node struct {
	Name        string
	RunFunc     func(ctx context.Context, s state.Writer) (*NodeResult, error)
	RetryPolicy *RetryPolicy // Optional retry configuration for this node
}

// NodeResult contains the output of a node execution.
type NodeResult struct {
	Updates  map[string]any    // State updates
	Messages []message.Message // Messages to append
}

// Run executes the node's function.
func (n *Node) Run(ctx context.Context, s state.Writer) (*NodeResult, error) {
	if n.RunFunc == nil {
		return nil, nil
	}
	return n.RunFunc(ctx, s)
}
