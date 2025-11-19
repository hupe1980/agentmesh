package graph

import (
	"fmt"
	"sort"
)

// NodeInfo contains metadata about a node in the graph.
type NodeInfo struct {
	Name              string `json:"name"`
	Type              string `json:"type"` // "standard", "start", "end"
	IncomingEdges     int    `json:"incoming_edges"`
	OutgoingEdges     int    `json:"outgoing_edges"`
	IsConditional     bool   `json:"is_conditional"`
	IsConditionalGate bool   `json:"is_conditional_gate"`
	HasRetryPolicy    bool   `json:"has_retry_policy"`
	RetryMaxAttempts  int    `json:"retry_max_attempts,omitempty"`
}

// EdgeInfo contains metadata about an edge in the graph.
type EdgeInfo struct {
	From               string   `json:"from"`
	To                 string   `json:"to"`
	Type               string   `json:"type"` // "direct", "conditional"
	ConditionalTargets []string `json:"conditional_targets,omitempty"`
}

// Topology provides a complete view of the graph structure.
type Topology struct {
	Nodes            []NodeInfo `json:"nodes"`
	Edges            []EdgeInfo `json:"edges"`
	EntryPoints      []string   `json:"entry_points"`
	ExitPoints       []string   `json:"exit_points"`
	ConditionalNodes []string   `json:"conditional_nodes"`
	IsolatedNodes    []string   `json:"isolated_nodes"`
	MaxDepth         int        `json:"max_depth"`
	TotalPaths       int        `json:"total_paths"`
}

// Metrics provides runtime execution metrics.
type Metrics struct {
	TotalNodes           int            `json:"total_nodes"`
	TotalEdges           int            `json:"total_edges"`
	ConditionalEdges     int            `json:"conditional_edges"`
	AverageFanOut        float64        `json:"average_fan_out"`
	MaxFanOut            int            `json:"max_fan_out"`
	AverageFanIn         float64        `json:"average_fan_in"`
	MaxFanIn             int            `json:"max_fan_in"`
	CyclomaticComplexity int            `json:"cyclomatic_complexity"`
	NodesByType          map[string]int `json:"nodes_by_type"`
}

// NodeDependencies describes a node's dependencies and dependents.
type NodeDependencies struct {
	Node               string   `json:"node"`
	DirectPredecessors []string `json:"direct_predecessors"`
	DirectSuccessors   []string `json:"direct_successors"`
	AllPredecessors    []string `json:"all_predecessors"`
	AllSuccessors      []string `json:"all_successors"`
	Depth              int      `json:"depth"` // Distance from START
}

// GetNodes returns a list of all node names in the graph.
func (g *Graph) GetNodes() []string {
	result := make([]string, 0, len(g.Nodes))
	for name := range g.Nodes {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// GetNodeInfo returns detailed information about a specific node.
//
//nolint:gocyclo // Acceptable complexity for comprehensive node introspection
func (g *Graph) GetNodeInfo(name string) (*NodeInfo, error) {
	node, exists := g.Nodes[name]
	if !exists {
		return nil, fmt.Errorf("node not found: %s", name)
	}

	// Count incoming edges
	incomingCount := 0
	for _, edge := range g.Edges {
		if edge.To == name {
			incomingCount++
		}
	}

	// Count outgoing edges
	outgoingCount := 0
	for _, edge := range g.Edges {
		if edge.From == name {
			outgoingCount++
		}
	}

	// Check if node has conditional edges
	hasConditional := false
	for _, ce := range g.Branches {
		if ce.From == name {
			hasConditional = true
			break
		}
	}

	// Check if node is a conditional gate (target of conditional edges)
	isConditionalGate := false
	for _, ce := range g.Branches {
		for _, target := range ce.Targets {
			if target == name {
				isConditionalGate = true
				break
			}
		}
		if isConditionalGate {
			break
		}
	}

	// Check if node supports retry policy
	var retryPolicy *RetryPolicy
	if retryNode, ok := node.(NodeWithRetry); ok {
		retryPolicy = retryNode.RetryPolicy()
	}

	info := &NodeInfo{
		Name:              name,
		Type:              "standard",
		IncomingEdges:     incomingCount,
		OutgoingEdges:     outgoingCount,
		IsConditional:     hasConditional,
		IsConditionalGate: isConditionalGate,
		HasRetryPolicy:    retryPolicy != nil,
	}

	switch name {
	case StartNode:
		info.Type = "start"
	case EndNode:
		info.Type = "end"
	}

	if retryPolicy != nil {
		info.RetryMaxAttempts = retryPolicy.MaxAttempts
	}

	return info, nil
}

// GetAllNodeInfo returns information about all nodes in the graph.
func (g *Graph) GetAllNodeInfo() []NodeInfo {
	result := make([]NodeInfo, 0, len(g.Nodes))
	for name := range g.Nodes {
		if info, err := g.GetNodeInfo(name); err == nil {
			result = append(result, *info)
		}
	}
	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// GetEdges returns information about all edges in the graph.
func (g *Graph) GetEdges() []EdgeInfo {
	edges := make([]EdgeInfo, 0)

	// Direct edges
	for _, edge := range g.Edges {
		edges = append(edges, EdgeInfo{
			From: edge.From,
			To:   edge.To,
			Type: "direct",
		})
	}

	// Conditional edges
	for _, ce := range g.Branches {
		targets := make([]string, len(ce.Targets))
		copy(targets, ce.Targets)
		sort.Strings(targets)

		edges = append(edges, EdgeInfo{
			From:               ce.From,
			To:                 "", // Conditional edges don't have a single "to"
			Type:               "conditional",
			ConditionalTargets: targets,
		})
	}

	return edges
}

// GetTopology returns a comprehensive view of the graph structure.
func (g *Graph) GetTopology() *Topology {
	topo := &Topology{
		Nodes:            g.GetAllNodeInfo(),
		Edges:            g.GetEdges(),
		EntryPoints:      make([]string, 0),
		ExitPoints:       make([]string, 0),
		ConditionalNodes: make([]string, 0),
		IsolatedNodes:    make([]string, 0),
	}

	// Find entry points (nodes with edges from START)
	for _, edge := range g.Edges {
		if edge.From == StartNode {
			topo.EntryPoints = append(topo.EntryPoints, edge.To)
		}
	}
	sort.Strings(topo.EntryPoints)

	// Find exit points (nodes with edges to END)
	for _, edge := range g.Edges {
		if edge.To == EndNode && edge.From != StartNode {
			topo.ExitPoints = append(topo.ExitPoints, edge.From)
		}
	}
	sort.Strings(topo.ExitPoints)

	// Find conditional nodes
	for _, ce := range g.Branches {
		topo.ConditionalNodes = append(topo.ConditionalNodes, ce.From)
	}
	sort.Strings(topo.ConditionalNodes)

	// Find isolated nodes (no incoming or outgoing edges)
	for name := range g.Nodes {
		hasIncoming := false
		hasOutgoing := false

		for _, edge := range g.Edges {
			if edge.To == name {
				hasIncoming = true
			}
			if edge.From == name {
				hasOutgoing = true
			}
		}

		if !hasIncoming && !hasOutgoing {
			topo.IsolatedNodes = append(topo.IsolatedNodes, name)
		}
	}
	sort.Strings(topo.IsolatedNodes)

	// Calculate max depth
	topo.MaxDepth = g.calculateMaxDepth()

	// Calculate total possible paths (estimate)
	topo.TotalPaths = g.estimateTotalPaths()

	return topo
}

// GetMetrics returns runtime metrics about the graph.
func (g *Graph) GetMetrics() *Metrics {
	metrics := &Metrics{
		TotalNodes:       len(g.Nodes),
		TotalEdges:       len(g.Edges),
		ConditionalEdges: len(g.Branches),
		NodesByType:      make(map[string]int),
	}

	// Build outgoing map for fan-out calculation
	outgoing := make(map[string][]string)
	for _, edge := range g.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
	}

	// Calculate fan-out statistics
	totalFanOut := 0
	for _, targets := range outgoing {
		fanOut := len(targets)
		totalFanOut += fanOut
		if fanOut > metrics.MaxFanOut {
			metrics.MaxFanOut = fanOut
		}
	}
	if len(outgoing) > 0 {
		metrics.AverageFanOut = float64(totalFanOut) / float64(len(outgoing))
	}

	// Build incoming count map for fan-in calculation
	incomingCount := make(map[string]int)
	for _, edge := range g.Edges {
		incomingCount[edge.To]++
	}

	// Calculate fan-in statistics
	totalFanIn := 0
	for _, count := range incomingCount {
		if count > metrics.MaxFanIn {
			metrics.MaxFanIn = count
		}
		totalFanIn += count
	}
	if len(incomingCount) > 0 {
		metrics.AverageFanIn = float64(totalFanIn) / float64(len(incomingCount))
	}

	// Count nodes by type
	for name := range g.Nodes {
		info, _ := g.GetNodeInfo(name)
		if info != nil {
			metrics.NodesByType[info.Type]++
		}
	}

	// Calculate cyclomatic complexity: E - N + 2P
	// E = edges, N = nodes, P = connected components (assume 1)
	metrics.CyclomaticComplexity = len(g.Edges) - len(g.Nodes) + 2

	return metrics
}

// GetNodeDependencies returns dependency information for a specific node.
func (g *Graph) GetNodeDependencies(name string) (*NodeDependencies, error) {
	if _, exists := g.Nodes[name]; !exists {
		return nil, fmt.Errorf("node not found: %s", name)
	}

	deps := &NodeDependencies{
		Node:               name,
		DirectPredecessors: make([]string, 0),
		DirectSuccessors:   make([]string, 0),
		AllPredecessors:    make([]string, 0),
		AllSuccessors:      make([]string, 0),
	}

	// Find direct predecessors (nodes with edges to this node)
	for _, edge := range g.Edges {
		if edge.To == name {
			deps.DirectPredecessors = append(deps.DirectPredecessors, edge.From)
		}
	}
	sort.Strings(deps.DirectPredecessors)

	// Find direct successors
	for _, edge := range g.Edges {
		if edge.From == name {
			deps.DirectSuccessors = append(deps.DirectSuccessors, edge.To)
		}
	}
	sort.Strings(deps.DirectSuccessors)

	// Find all predecessors (recursive)
	deps.AllPredecessors = g.findAllPredecessors(name)
	sort.Strings(deps.AllPredecessors)

	// Find all successors (recursive)
	deps.AllSuccessors = g.findAllSuccessors(name)
	sort.Strings(deps.AllSuccessors)

	// Calculate depth from START
	deps.Depth = g.calculateDepth(name)

	return deps, nil
}

// Helper functions

func (g *Graph) calculateMaxDepth() int {
	maxDepth := 0
	for name := range g.Nodes {
		depth := g.calculateDepth(name)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

func (g *Graph) calculateDepth(name string) int {
	if name == StartNode {
		return 0
	}

	visited := make(map[string]bool)
	return g.calculateDepthRecursive(name, visited)
}

func (g *Graph) calculateDepthRecursive(name string, visited map[string]bool) int {
	if name == StartNode {
		return 0
	}

	if visited[name] {
		return 0 // Cycle detected, stop
	}
	visited[name] = true

	maxPredDepth := -1
	for _, edge := range g.Edges {
		if edge.To == name {
			predDepth := g.calculateDepthRecursive(edge.From, visited)
			if predDepth > maxPredDepth {
				maxPredDepth = predDepth
			}
		}
	}

	if maxPredDepth < 0 {
		return 0
	}
	return maxPredDepth + 1
}

func (g *Graph) estimateTotalPaths() int {
	// Simple estimation: count branches
	paths := 1
	for _, ce := range g.Branches {
		if len(ce.Targets) > 1 {
			paths *= len(ce.Targets)
		}
	}
	return paths
}

func (g *Graph) findAllPredecessors(name string) []string {
	visited := make(map[string]bool)
	result := make([]string, 0)
	g.findAllPredecessorsRecursive(name, visited, &result)
	return result
}

func (g *Graph) findAllPredecessorsRecursive(name string, visited map[string]bool, result *[]string) {
	if visited[name] {
		return
	}
	visited[name] = true

	for _, edge := range g.Edges {
		if edge.To == name && edge.From != StartNode {
			*result = append(*result, edge.From)
			g.findAllPredecessorsRecursive(edge.From, visited, result)
		}
	}
}

func (g *Graph) findAllSuccessors(name string) []string {
	visited := make(map[string]bool)
	result := make([]string, 0)
	g.findAllSuccessorsRecursive(name, visited, &result)
	return result
}

func (g *Graph) findAllSuccessorsRecursive(name string, visited map[string]bool, result *[]string) {
	if visited[name] {
		return
	}
	visited[name] = true

	for _, edge := range g.Edges {
		if edge.From == name && edge.To != EndNode {
			*result = append(*result, edge.To)
			g.findAllSuccessorsRecursive(edge.To, visited, result)
		}
	}
}
