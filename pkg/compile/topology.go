package compile

import (
	"sort"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// executionTopology holds computed graph topology information.
// This is created during compilation and used internally by executors.
// It's optimized for fast lookups during execution, not for user-facing introspection.
type executionTopology struct {
	Edges             []graph.Edge                        // Deduplicated edges
	Incoming          map[string]int                      // Incoming edge count per node
	Outgoing          map[string][]string                 // Outgoing edges per node
	ConditionalGate   map[string]bool                     // Nodes behind conditional gates
	ConditionalByFrom map[string][]graph.ConditionalEdges // Conditional edges grouped by source
	NodeNames         []string                            // Sorted node names
}

// computeTopology analyzes the graph structure and builds topology metadata.
// This computes incoming/outgoing edges, identifies conditional gates, and
// prepares data structures needed for execution.
func computeTopology(nodes map[string]graph.Node, edges []graph.Edge, conditionals []graph.ConditionalEdges) *executionTopology {
	topo := &executionTopology{
		Incoming:          make(map[string]int, len(nodes)),
		Outgoing:          make(map[string][]string),
		ConditionalGate:   make(map[string]bool),
		ConditionalByFrom: make(map[string][]graph.ConditionalEdges),
	}

	// Initialize incoming count for all nodes
	for name := range nodes {
		topo.Incoming[name] = 0
	}

	// Deduplicate edges
	type edgeKey struct {
		from string
		to   string
	}
	seen := make(map[edgeKey]struct{})

	for _, edge := range edges {
		if edge.From == "" || edge.To == "" {
			continue
		}

		key := edgeKey{from: edge.From, to: edge.To}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		// Add to edges list and outgoing map
		topo.Edges = append(topo.Edges, edge)
		topo.Outgoing[edge.From] = append(topo.Outgoing[edge.From], edge.To)

		// Track incoming edges (excluding special __end__ handling)
		if edge.To != "__end__" {
			if _, ok := topo.Incoming[edge.To]; !ok {
				topo.Incoming[edge.To] = 0
			}
			if edge.From != "__start__" {
				topo.Incoming[edge.To]++
			}
		}
	}

	// Process conditional edges
	for _, cond := range conditionals {
		if len(cond.Targets) == 0 {
			continue
		}

		// Clone targets to avoid mutation
		targets := make([]string, len(cond.Targets))
		copy(targets, cond.Targets)

		topo.ConditionalByFrom[cond.From] = append(
			topo.ConditionalByFrom[cond.From],
			graph.ConditionalEdges{
				From:      cond.From,
				Targets:   targets,
				Condition: cond.Condition,
			},
		)

		// Mark nodes behind conditional gates
		for _, target := range targets {
			if target == "" || target == "__end__" {
				continue
			}
			topo.ConditionalGate[target] = true
		}
	}

	// Generate sorted node names for deterministic iteration
	topo.NodeNames = make([]string, 0, len(nodes))
	for name := range nodes {
		topo.NodeNames = append(topo.NodeNames, name)
	}
	sort.Strings(topo.NodeNames)

	return topo
}
