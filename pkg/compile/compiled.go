package compile

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Reserved node names
const (
	// StartNode is the graph entry point
	StartNode = "__start__"

	// EndNode is the graph exit point
	EndNode = "__end__"
)

// CompiledGraph represents a validated and compiled graph ready for execution.
// It contains the original graph structure plus computed topology information.
type CompiledGraph struct {
	// Original graph structure
	Graph *graph.Graph

	// Computed topology (internal, optimized for execution)
	Topology *executionTopology

	// State manager for execution
	StateManager state.StateManager

	// Quick lookups
	StartNode string
	EndNode   string
}

// Compile takes a graph and compiles it into an executable form.
// This validates the graph structure and computes topology information.
func Compile(g *graph.Graph, stateManager state.StateManager) (*CompiledGraph, error) {
	// TODO: Add validation
	// - Check for cycles
	// - Verify all edge endpoints exist
	// - Validate start/end nodes

	// Compute topology (graph.Graph uses Branches, not Conditionals)
	topo := computeTopology(g.Nodes, g.Edges, g.Branches)

	return &CompiledGraph{
		Graph:        g,
		Topology:     topo,
		StateManager: stateManager,
		StartNode:    StartNode, // Use constant, not field
		EndNode:      EndNode,   // Use constant, not field
	}, nil
}

// Nodes returns all nodes in the graph.
func (cg *CompiledGraph) Nodes() map[string]*graph.Node {
	return cg.Graph.Nodes
}

// GetNode returns a node by name, or nil if not found.
func (cg *CompiledGraph) GetNode(name string) *graph.Node {
	return cg.Graph.Nodes[name]
}
