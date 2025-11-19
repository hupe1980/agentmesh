package graph

import (
	"sort"
)

// topology holds computed graph topology information for execution.
// This is internal and not exposed to users.
type topology struct {
	edges             []Edge                        // Deduplicated edges
	incoming          map[string]int                // Incoming edge count per node
	outgoing          map[string][]string           // Outgoing edges per node
	conditionalGate   map[string]bool               // Nodes behind conditional gates
	conditionalByFrom map[int][]ConditionalEdges    // Conditional edges grouped by source index
	nodeNames         []string                      // Sorted node names
}

// computeTopology analyzes the graph structure and builds topology metadata.
// This computes incoming/outgoing edges, identifies conditional gates, and
// prepares data structures needed for execution.
func computeTopology(nodes map[string]Node, edges []Edge, conditionals []ConditionalEdges) *topology {
	topo := &topology{
		incoming:          make(map[string]int, len(nodes)),
		outgoing:          make(map[string][]string),
		conditionalGate:   make(map[string]bool),
		conditionalByFrom: make(map[int][]ConditionalEdges),
	}

	// Initialize incoming count for all nodes
	for name := range nodes {
		topo.incoming[name] = 0
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
		topo.edges = append(topo.edges, edge)
		topo.outgoing[edge.From] = append(topo.outgoing[edge.From], edge.To)

		// Track incoming edges (excluding special __end__ handling)
		if edge.To != "__end__" {
			if _, ok := topo.incoming[edge.To]; !ok {
				topo.incoming[edge.To] = 0
			}
			if edge.From != "__start__" {
				topo.incoming[edge.To]++
			}
		}
	}

	// Process conditional edges
	for i, cond := range conditionals {
		if len(cond.Targets) == 0 {
			continue
		}

		// Clone targets to avoid mutation
		targets := make([]string, len(cond.Targets))
		copy(targets, cond.Targets)

		topo.conditionalByFrom[i] = append(
			topo.conditionalByFrom[i],
			ConditionalEdges{
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
			topo.conditionalGate[target] = true
		}
	}

	// Generate sorted node names for deterministic iteration
	topo.nodeNames = make([]string, 0, len(nodes))
	for name := range nodes {
		topo.nodeNames = append(topo.nodeNames, name)
	}
	sort.Strings(topo.nodeNames)

	return topo
}
