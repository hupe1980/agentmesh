package graph

import (
	"fmt"
	"sort"
	"strings"
)

// NodeInfo contains metadata about a node in the graph.
type NodeInfo struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"` // "standard", "entry", "terminal"
	IncomingEdges   int      `json:"incoming_edges"`
	OutgoingEdges   int      `json:"outgoing_edges"`
	DeclaredTargets []string `json:"declared_targets,omitempty"`
	IsEntryPoint    bool     `json:"is_entry_point"`
	HasInterrupt    bool     `json:"has_interrupt"`
}

// EdgeInfo contains metadata about an edge in the graph.
type EdgeInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Topology provides a complete view of the graph structure.
type Topology struct {
	Nodes       []NodeInfo `json:"nodes"`
	Edges       []EdgeInfo `json:"edges"`
	EntryPoints []string   `json:"entry_points"`
	ExitPoints  []string   `json:"exit_points"` // Nodes that can route to END
}

// Metrics provides static graph metrics.
type Metrics struct {
	TotalNodes           int            `json:"total_nodes"`
	TotalEdges           int            `json:"total_edges"`
	AverageFanOut        float64        `json:"average_fan_out"`
	MaxFanOut            int            `json:"max_fan_out"`
	AverageFanIn         float64        `json:"average_fan_in"`
	MaxFanIn             int            `json:"max_fan_in"`
	CyclomaticComplexity int            `json:"cyclomatic_complexity"`
	NodesByType          map[string]int `json:"nodes_by_type"`
}

// GetNodes returns a sorted list of all node names in the graph.
func (g *Graph[I, O]) GetNodes() []string {
	names := make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetNodeInfo returns detailed information about a specific node.
func (g *Graph[I, O]) GetNodeInfo(name string) (*NodeInfo, error) {
	n, ok := g.nodes[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, name)
	}

	// Check if entry point
	isEntry := false
	for _, ep := range g.entryPoints {
		if ep == name {
			isEntry = true
			break
		}
	}

	// Check for interrupts
	hasInterrupt := false
	for _, ip := range g.interrupts {
		if ip.nodeName == name {
			hasInterrupt = true
			break
		}
	}

	// Count incoming edges
	incoming := 0
	if isEntry {
		incoming++ // START -> node
	}
	for _, other := range g.nodes {
		for _, t := range other.targets {
			if t == name {
				incoming++
			}
		}
	}

	// Determine type
	nodeType := "standard"
	if isEntry {
		nodeType = "entry"
	}
	for _, t := range n.targets {
		if t == END {
			nodeType = "terminal"
			break
		}
	}

	return &NodeInfo{
		Name:            name,
		Type:            nodeType,
		IncomingEdges:   incoming,
		OutgoingEdges:   len(n.targets),
		DeclaredTargets: n.targets,
		IsEntryPoint:    isEntry,
		HasInterrupt:    hasInterrupt,
	}, nil
}

// GetTopology returns a comprehensive view of the graph structure.
func (g *Graph[I, O]) GetTopology() *Topology {
	nodes := make([]NodeInfo, 0, len(g.nodes))
	edges := make([]EdgeInfo, 0)
	exitPoints := make([]string, 0)

	for name := range g.nodes {
		info, _ := g.GetNodeInfo(name)
		if info != nil {
			nodes = append(nodes, *info)
		}
	}

	// Sort nodes by name for consistent output
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	// Collect edges and exit points
	for _, ep := range g.entryPoints {
		edges = append(edges, EdgeInfo{From: "__start__", To: ep})
	}

	for name, n := range g.nodes {
		for _, target := range n.targets {
			edges = append(edges, EdgeInfo{From: name, To: target})
			if target == END {
				exitPoints = append(exitPoints, name)
			}
		}
	}

	sort.Strings(exitPoints)

	return &Topology{
		Nodes:       nodes,
		Edges:       edges,
		EntryPoints: g.entryPoints,
		ExitPoints:  exitPoints,
	}
}

// GetMetrics returns static graph metrics.
func (g *Graph[I, O]) GetMetrics() *Metrics {
	totalNodes := len(g.nodes)
	totalEdges := len(g.entryPoints) // START -> entry edges

	fanOuts := make([]int, 0, totalNodes)
	fanIns := make(map[string]int)

	// Initialize fanIns for all nodes
	for name := range g.nodes {
		fanIns[name] = 0
	}

	// Count entry point edges
	for _, ep := range g.entryPoints {
		fanIns[ep]++
	}

	// Count edges and fan-in/fan-out
	for _, n := range g.nodes {
		fanOut := len(n.targets)
		fanOuts = append(fanOuts, fanOut)
		totalEdges += fanOut

		for _, target := range n.targets {
			if target != END {
				fanIns[target]++
			}
		}
	}

	// Calculate averages and max
	var avgFanOut, avgFanIn float64
	var maxFanOut, maxFanIn int

	if totalNodes > 0 {
		sumFanOut := 0
		for _, fo := range fanOuts {
			sumFanOut += fo
			if fo > maxFanOut {
				maxFanOut = fo
			}
		}
		avgFanOut = float64(sumFanOut) / float64(totalNodes)

		sumFanIn := 0
		for _, fi := range fanIns {
			sumFanIn += fi
			if fi > maxFanIn {
				maxFanIn = fi
			}
		}
		avgFanIn = float64(sumFanIn) / float64(totalNodes)
	}

	// Cyclomatic complexity: E - N + 2P (P=1 for single connected graph)
	cyclomaticComplexity := totalEdges - totalNodes + 2

	// Count nodes by type
	nodesByType := map[string]int{
		"standard": 0,
		"entry":    0,
		"terminal": 0,
	}
	for name := range g.nodes {
		info, _ := g.GetNodeInfo(name)
		if info != nil {
			nodesByType[info.Type]++
		}
	}

	return &Metrics{
		TotalNodes:           totalNodes,
		TotalEdges:           totalEdges,
		AverageFanOut:        avgFanOut,
		MaxFanOut:            maxFanOut,
		AverageFanIn:         avgFanIn,
		MaxFanIn:             maxFanIn,
		CyclomaticComplexity: cyclomaticComplexity,
		NodesByType:          nodesByType,
	}
}

// MermaidFlowchart generates a Mermaid flowchart representation.
// Direction can be "TD" (top-down), "LR" (left-right), "BT", "RL".
func (g *Graph[I, O]) MermaidFlowchart(direction string) string {
	if direction == "" {
		direction = "TD"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("graph %s\n", direction))

	// Add START and END nodes
	sb.WriteString("    __start__([START])\n")
	sb.WriteString("    __end__([END])\n")

	// Add regular nodes
	for name, n := range g.nodes {
		// Use diamond shape for nodes with multiple targets (branching)
		if len(n.targets) > 1 {
			sb.WriteString(fmt.Sprintf("    %s{%s}\n", name, name))
		} else {
			sb.WriteString(fmt.Sprintf("    %s[%s]\n", name, name))
		}
	}

	// Add entry point edges
	for _, ep := range g.entryPoints {
		sb.WriteString(fmt.Sprintf("    __start__ --> %s\n", ep))
	}

	// Add node edges (dashed for dynamic routing)
	for name, n := range g.nodes {
		for _, target := range n.targets {
			sb.WriteString(fmt.Sprintf("    %s -.-> %s\n", name, target))
		}
	}

	return sb.String()
}
