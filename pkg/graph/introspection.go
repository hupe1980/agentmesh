package graph

import (
	"fmt"
	"slices"
	"sort"
)

// NodeInfo contains metadata about a node in the graph.
type NodeInfo struct {
	Name              string   `json:"name"`
	Type              string   `json:"type"` // "standard", "start", "end"
	IncomingEdges     int      `json:"incoming_edges"`
	OutgoingEdges     int      `json:"outgoing_edges"`
	HasRetryPolicy    bool     `json:"has_retry_policy"`
	RetryMaxAttempts  int      `json:"retry_max_attempts,omitempty"`
	DeclaredTargets   []string `json:"declared_targets,omitempty"`    // Command pattern: possible routing targets
	HasCommandRouting bool     `json:"has_command_routing,omitempty"` // True if node uses Command pattern
}

// EdgeInfo contains metadata about an edge in the graph.
type EdgeInfo struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Type           string   `json:"type"`                      // "direct", "command"
	CommandTargets []string `json:"command_targets,omitempty"` // Command pattern: declared routing targets
}

// Topology provides a complete view of the graph structure.
type Topology struct {
	Nodes         []NodeInfo `json:"nodes"`
	Edges         []EdgeInfo `json:"edges"`
	EntryPoints   []string   `json:"entry_points"`
	ExitPoints    []string   `json:"exit_points"`
	CommandNodes  []string   `json:"command_nodes"` // Nodes using Command pattern
	IsolatedNodes []string   `json:"isolated_nodes"`
	MaxDepth      int        `json:"max_depth"`
	TotalPaths    int        `json:"total_paths"`
}

// Metrics provides runtime execution metrics.
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
func (g *Graph) GetNodeInfo(nodeName string) (*NodeInfo, error) {
	node, exists := g.Nodes[nodeName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, nodeName)
	}

	// Count incoming edges from entry points and DeclaredTargets
	incomingCount := 0
	// Check implicit edges from START to EntryPoints
	for _, ep := range g.EntryPoints {
		if ep == nodeName {
			incomingCount++
			break
		}
	}
	// Check DeclaredTargets from all nodes
	for _, n := range g.Nodes {
		targets := n.Targets()
		for _, target := range targets {
			if target == nodeName {
				incomingCount++
			}
		}
	}

	// Count outgoing edges from this node's DeclaredTargets
	outgoingCount := len(node.Targets())

	// Check if node supports retry policy
	var retryPolicy *RetryPolicy
	if retryNode, ok := node.(NodeWithRetry); ok {
		retryPolicy = retryNode.RetryPolicy()
	}

	info := &NodeInfo{
		Name:           nodeName,
		Type:           "standard",
		IncomingEdges:  incomingCount,
		OutgoingEdges:  outgoingCount,
		HasRetryPolicy: retryPolicy != nil,
	}

	switch nodeName {
	case StartNode:
		info.Type = "start"
	case EndNode:
		info.Type = "end"
	}

	if retryPolicy != nil {
		info.RetryMaxAttempts = retryPolicy.MaxAttempts
	}

	// Get declared targets from Command nodes
	targets := node.Targets()
	if len(targets) > 0 {
		info.DeclaredTargets = targets
		info.HasCommandRouting = true
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

	// Direct edges from START to all entry points
	for _, entryPoint := range g.EntryPoints {
		edges = append(edges, EdgeInfo{
			From: StartNode,
			To:   entryPoint,
			Type: "direct",
		})
	}

	// Command pattern routing (nodes define their targets via DeclaredTargets)
	for name, node := range g.Nodes {
		targets := node.Targets()
		if len(targets) > 0 {
			cmdTargets := make([]string, len(targets))
			copy(cmdTargets, targets)
			sort.Strings(cmdTargets)

			edges = append(edges, EdgeInfo{
				From:           name,
				To:             "", // Command routing is dynamic
				Type:           "command",
				CommandTargets: cmdTargets,
			})
		}
	}

	return edges
}

// GetTopology returns a comprehensive view of the graph structure.
func (g *Graph) GetTopology() *Topology {
	topo := &Topology{
		Nodes:         g.GetAllNodeInfo(),
		Edges:         g.GetEdges(),
		EntryPoints:   make([]string, 0),
		ExitPoints:    make([]string, 0),
		CommandNodes:  make([]string, 0),
		IsolatedNodes: make([]string, 0),
	}

	// Entry points are explicitly defined
	topo.EntryPoints = make([]string, len(g.EntryPoints))
	copy(topo.EntryPoints, g.EntryPoints)
	sort.Strings(topo.EntryPoints)

	// Find exit points (nodes that target END in their DeclaredTargets)
	for name, node := range g.Nodes {
		targets := node.Targets()
		if slices.Contains(targets, EndNode) {
			topo.ExitPoints = append(topo.ExitPoints, name)
		}
	}
	sort.Strings(topo.ExitPoints)

	// Command pattern: no conditional edges, all routing via DeclaredTargets
	// ConditionalNodes field kept for backward compatibility but will be empty

	// Find Command nodes (nodes with declared targets)
	for name, node := range g.Nodes {
		if len(node.Targets()) > 0 {
			topo.CommandNodes = append(topo.CommandNodes, name)
		}
	}
	sort.Strings(topo.CommandNodes)

	// Find isolated nodes (no incoming or outgoing edges/targets)
	for name := range g.Nodes {
		hasIncoming := false
		hasOutgoing := false

		// Check if any node targets this node in DeclaredTargets
		if !hasIncoming {
			for _, node := range g.Nodes {
				targets := node.Targets()
				if slices.Contains(targets, name) {
					hasIncoming = true
				}
				if hasIncoming {
					break
				}
			}
		}

		// Check if this node has DeclaredTargets
		if !hasOutgoing {
			if len(g.Nodes[name].Targets()) > 0 {
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
//
//nolint:gocyclo // Acceptable complexity for comprehensive metrics calculation
func (g *Graph) GetMetrics() *Metrics {
	// Count total edges: implicit START->EntryPoints + DeclaredTargets
	totalEdges := len(g.EntryPoints) // START -> each entry point
	for _, node := range g.Nodes {
		totalEdges += len(node.Targets())
	}

	metrics := &Metrics{
		TotalNodes:  len(g.Nodes),
		TotalEdges:  totalEdges,
		NodesByType: make(map[string]int),
	}

	// Build outgoing map for fan-out calculation (include DeclaredTargets)
	outgoing := make(map[string][]string)
	// Add implicit START -> EntryPoints edges
	if len(g.EntryPoints) > 0 {
		outgoing[StartNode] = make([]string, len(g.EntryPoints))
		copy(outgoing[StartNode], g.EntryPoints)
	}
	for name, node := range g.Nodes {
		targets := node.Targets()
		if len(targets) > 0 {
			outgoing[name] = append(outgoing[name], targets...)
		}
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

	// Build incoming count map for fan-in calculation (include DeclaredTargets)
	incomingCount := make(map[string]int)
	// Implicit START -> EntryPoints
	for _, ep := range g.EntryPoints {
		incomingCount[ep]++
	}
	for _, node := range g.Nodes {
		for _, target := range node.Targets() {
			incomingCount[target]++
		}
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
	metrics.CyclomaticComplexity = totalEdges - len(g.Nodes) + 2

	return metrics
}

// GetNodeDependencies returns dependency information for a specific node.
func (g *Graph) GetNodeDependencies(name string) (*NodeDependencies, error) {
	if _, exists := g.Nodes[name]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, name)
	}

	deps := &NodeDependencies{
		Node:               name,
		DirectPredecessors: make([]string, 0),
		DirectSuccessors:   make([]string, 0),
		AllPredecessors:    make([]string, 0),
		AllSuccessors:      make([]string, 0),
	}

	// Find direct predecessors (nodes with edges to this node, including DeclaredTargets)
	// Implicit edges from START to EntryPoints
	for _, ep := range g.EntryPoints {
		if ep == name {
			deps.DirectPredecessors = append(deps.DirectPredecessors, StartNode)
			break
		}
	}
	// Check DeclaredTargets from all nodes
	for nodeName, node := range g.Nodes {
		for _, target := range node.Targets() {
			if target == name {
				deps.DirectPredecessors = append(deps.DirectPredecessors, nodeName)
			}
		}
	}
	sort.Strings(deps.DirectPredecessors)

	// Find direct successors (from this node's DeclaredTargets)
	if node, exists := g.Nodes[name]; exists {
		deps.DirectSuccessors = append(deps.DirectSuccessors, node.Targets()...)
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

	// Check implicit edges from START to EntryPoints
	isEntryPoint := false
	for _, ep := range g.EntryPoints {
		if ep == name {
			isEntryPoint = true
			break
		}
	}
	if isEntryPoint {
		predDepth := g.calculateDepthRecursive(StartNode, visited)
		if predDepth > maxPredDepth {
			maxPredDepth = predDepth
		}
	}

	// Check DeclaredTargets from all nodes
	for nodeName, node := range g.Nodes {
		for _, target := range node.Targets() {
			if target == name {
				predDepth := g.calculateDepthRecursive(nodeName, visited)
				if predDepth > maxPredDepth {
					maxPredDepth = predDepth
				}
			}
		}
	}

	if maxPredDepth < 0 {
		return 0
	}
	return maxPredDepth + 1
}

func (g *Graph) estimateTotalPaths() int {
	// Simple estimation: count Command nodes with multiple declared targets
	paths := 1

	// Count Command nodes with multiple DeclaredTargets
	for _, node := range g.Nodes {
		targets := node.Targets()
		if len(targets) > 1 {
			paths *= len(targets)
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

	// Check DeclaredTargets from all nodes (including implicit START->EntryPoint)
	for nodeName, node := range g.Nodes {
		for _, target := range node.Targets() {
			if target == name && nodeName != StartNode {
				*result = append(*result, nodeName)
				g.findAllPredecessorsRecursive(nodeName, visited, result)
			}
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

	// Check this node's DeclaredTargets
	if node, exists := g.Nodes[name]; exists {
		for _, target := range node.Targets() {
			if target != EndNode {
				*result = append(*result, target)
				g.findAllSuccessorsRecursive(target, visited, result)
			}
		}
	}
}
