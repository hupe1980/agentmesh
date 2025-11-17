package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Graph represents a mutable computational graph with nodes and edges.
type Graph struct {
	Nodes    map[string]*Node
	Edges    []Edge
	Branches []ConditionalEdges
	state    *state.State
}

// NewGraph creates a new graph with the given state.
func NewGraph(st *state.State) (*Graph, error) {
	return &Graph{
		Nodes:    make(map[string]*Node),
		Edges:    make([]Edge, 0),
		Branches: make([]ConditionalEdges, 0),
		state:    st,
	}, nil
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(n *Node) error {
	if n == nil {
		return fmt.Errorf("node cannot be nil")
	}
	if n.Name == "" {
		return fmt.Errorf("node name cannot be empty")
	}
	g.Nodes[n.Name] = n
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

// State returns the graph's state.
func (g *Graph) State() *state.State {
	return g.state
}
