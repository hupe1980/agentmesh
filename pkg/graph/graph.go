package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Graph represents a mutable computational graph with nodes and edges.
type Graph struct {
	Nodes    map[string]Node
	Edges    []Edge
	Branches []ConditionalEdges
	manager  *state.Manager
}

// NewGraph creates a new graph with the given state manager.
func NewGraph(manager *state.Manager) (*Graph, error) {
	if manager == nil {
		return nil, fmt.Errorf("manager cannot be nil")
	}
	return &Graph{
		Nodes:    make(map[string]Node),
		Edges:    make([]Edge, 0),
		Branches: make([]ConditionalEdges, 0),
		manager:  manager,
	}, nil
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(n Node) error {
	if n == nil {
		return fmt.Errorf("node cannot be nil")
	}
	if n.Name() == "" {
		return fmt.Errorf("node name cannot be empty")
	}
	g.Nodes[n.Name()] = n
	return nil
}

// SetNodeRetryPolicy sets or updates the retry policy for an existing node.
// Returns an error if the node doesn't exist or doesn't support retry.
func (g *Graph) SetNodeRetryPolicy(name string, retryPolicy *RetryPolicy) error {
	node, exists := g.Nodes[name]
	if !exists {
		return fmt.Errorf("node not found: %s", name)
	}

	// Only BaseNode supports setting retry policy after creation
	baseNode, ok := node.(*BaseNode)
	if !ok {
		return fmt.Errorf("node %s does not support setting retry policy", name)
	}

	baseNode.retryPolicy = retryPolicy
	return nil
}

// AddEdge adds a directed edge between two nodes.
func (g *Graph) AddEdge(from, to string) {
	g.Edges = append(g.Edges, Edge{From: from, To: to})
}

// AddConditionalEdges adds conditional routing based on runtime state.
// The condition function receives a ReadView and returns target node names.
func (g *Graph) AddConditionalEdges(from string, condition func(context.Context, *state.ReadView) []string, targets []string) {
	g.Branches = append(g.Branches, ConditionalEdges{
		From:      from,
		Condition: condition,
		Targets:   targets,
	})
}

// Manager returns the graph's state manager.
func (g *Graph) Manager() *state.Manager {
	return g.manager
}
