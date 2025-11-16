package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Graph represents a mutable computational graph with nodes and edges.
type Graph struct {
	Nodes        map[string]*Node
	Edges        []Edge
	Branches     []ConditionalEdges
	stateManager state.StateManager
}

// NewGraph creates a new graph with the given state manager.
func NewGraph(stateManager state.StateManager) (*Graph, error) {
	return &Graph{
		Nodes:        make(map[string]*Node),
		Edges:        make([]Edge, 0),
		Branches:     make([]ConditionalEdges, 0),
		stateManager: stateManager,
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
func (g *Graph) AddConditionalEdges(from string, condition func(context.Context, state.Reader) []string, targets []string) {
	g.Branches = append(g.Branches, ConditionalEdges{
		From:      from,
		Condition: condition,
		Targets:   targets,
	})
}

// StateManager returns the graph's state manager.
func (g *Graph) StateManager() state.StateManager {
	return g.stateManager
}

// Compile compiles the graph into a MessageRunnable.
// This method exists for API compatibility. The actual compilation
// is handled by pkg/compile and execution by pkg/exec.
// To use this, call exec.CompileGraph() from pkg/exec.
func (g *Graph) Compile() (MessageRunnable, error) {
	// This cannot be implemented here due to import cycles.
	// Users must call exec.CompileGraph(g) instead.
	return nil, fmt.Errorf("graph.Compile() is deprecated; use exec.CompileGraph(graph) instead")
}
