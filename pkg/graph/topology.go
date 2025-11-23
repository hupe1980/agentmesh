package graph

import (
	"sort"
)

// topology holds computed graph topology information for execution.
// This is internal and not exposed to users.
type topology struct {
	incoming       map[string]int      // Incoming edge count per node
	outgoing       map[string][]string // Outgoing edges per node
	nodeNames      []string            // Sorted node names
	triggerToNodes map[string][]string // Nodes that can be triggered by each node's output
}

// computeTopology analyzes the graph structure and builds topology metadata.
// This computes incoming/outgoing edges and prepares data structures needed for execution.
func computeTopology(nodes map[string]Node, entryPoint string) *topology {
	topo := &topology{
		incoming: make(map[string]int, len(nodes)),
		outgoing: make(map[string][]string),
	}

	// Initialize incoming count for all nodes
	for name := range nodes {
		topo.incoming[name] = 0
	}

	// Process entry point (START -> entryPoint)
	if entryPoint != "" {
		topo.outgoing[StartNode] = append(topo.outgoing[StartNode], entryPoint)
		if entryPoint != EndNode && entryPoint != "" {
			if _, ok := topo.incoming[entryPoint]; !ok {
				topo.incoming[entryPoint] = 0
			}
		}
	}

	// Generate sorted node names for deterministic iteration
	topo.nodeNames = make([]string, 0, len(nodes))
	for name := range nodes {
		topo.nodeNames = append(topo.nodeNames, name)
	}
	sort.Strings(topo.nodeNames)

	// Compute triggerToNodes mapping (inverse of outgoing)
	// This maps each node to the list of nodes that can be triggered by its execution
	topo.triggerToNodes = make(map[string][]string)
	for from, targets := range topo.outgoing {
		if len(targets) > 0 {
			topo.triggerToNodes[from] = targets
		}
	}

	return topo
}
