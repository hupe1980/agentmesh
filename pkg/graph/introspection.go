package graph

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
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

// GraphTopology provides a complete view of the graph structure.
type GraphTopology struct {
	Nodes            []NodeInfo `json:"nodes"`
	Edges            []EdgeInfo `json:"edges"`
	EntryPoints      []string   `json:"entry_points"`
	ExitPoints       []string   `json:"exit_points"`
	ConditionalNodes []string   `json:"conditional_nodes"`
	IsolatedNodes    []string   `json:"isolated_nodes"`
	MaxDepth         int        `json:"max_depth"`
	TotalPaths       int        `json:"total_paths"`
}

// GraphMetrics provides runtime execution metrics.
type GraphMetrics struct {
	TotalNodes           int            `json:"total_nodes"`
	TotalEdges           int            `json:"total_edges"`
	ConditionalEdges     int            `json:"conditional_edges"`
	AverageFanOut        float64        `json:"average_fan_out"`
	MaxFanOut            int            `json:"max_fan_out"`
	AverageFanIn         float64        `json:"average_fan_in"`
	MaxFanIn             int            `json:"max_fan_in"`
	CyclomaticComplexity int            `json:"cyclomatic_complexity"`
	NodesByType          map[string]int `json:"nodes_by_type"`
	CurrentSuperstep     int64          `json:"current_superstep"`
	CompletedNodes       []string       `json:"completed_nodes"`
	PausedNodes          []string       `json:"paused_nodes"`
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
func (cg *CompiledGraph) GetNodes() []string {
	result := make([]string, len(cg.nodeNames))
	copy(result, cg.nodeNames)
	return result
}

// GetNodeInfo returns detailed information about a specific node.
func (cg *CompiledGraph) GetNodeInfo(name string) (*NodeInfo, error) {
	node, exists := cg.nodes[name]
	if !exists {
		return nil, ErrNodeNotFound
	}

	info := &NodeInfo{
		Name:              name,
		Type:              "standard",
		IncomingEdges:     cg.incoming[name],
		OutgoingEdges:     len(cg.outgoing[name]),
		IsConditional:     len(cg.conditionalByFrom[name]) > 0,
		IsConditionalGate: cg.conditionalGate[name],
		HasRetryPolicy:    node.RetryPolicy != nil,
	}

	switch name {
	case StartNode:
		info.Type = "start"
	case EndNode:
		info.Type = "end"
	}

	if node.RetryPolicy != nil {
		info.RetryMaxAttempts = node.RetryPolicy.MaxAttempts
	}

	return info, nil
}

// GetAllNodeInfo returns information about all nodes in the graph.
func (cg *CompiledGraph) GetAllNodeInfo() []NodeInfo {
	infos := make([]NodeInfo, 0, len(cg.nodes))

	for _, name := range cg.nodeNames {
		if info, err := cg.GetNodeInfo(name); err == nil {
			infos = append(infos, *info)
		}
	}

	return infos
}

// GetEdges returns all edges in the graph.
func (cg *CompiledGraph) GetEdges() []EdgeInfo {
	edges := make([]EdgeInfo, 0, len(cg.edges))

	// Add direct edges
	for _, edge := range cg.edges {
		edges = append(edges, EdgeInfo{
			From: edge.From,
			To:   edge.To,
			Type: "direct",
		})
	}

	// Add conditional edges
	for _, conditional := range cg.conditionals {
		edges = append(edges, EdgeInfo{
			From:               conditional.From,
			To:                 "", // Conditional edge doesn't have single target
			Type:               "conditional",
			ConditionalTargets: append([]string(nil), conditional.Targets...),
		})
	}

	return edges
}

// GetTopology returns a comprehensive view of the graph structure.
func (cg *CompiledGraph) GetTopology() *GraphTopology {
	topo := &GraphTopology{
		Nodes:            cg.GetAllNodeInfo(),
		Edges:            cg.GetEdges(),
		EntryPoints:      make([]string, 0),
		ExitPoints:       make([]string, 0),
		ConditionalNodes: make([]string, 0),
		IsolatedNodes:    make([]string, 0),
	}

	// Find entry points (nodes with edges from START)
	topo.EntryPoints = append(topo.EntryPoints, cg.outgoing[StartNode]...)
	sort.Strings(topo.EntryPoints)

	// Find exit points (nodes with edges to END)
	for _, edge := range cg.edges {
		if edge.To == EndNode && edge.From != StartNode {
			topo.ExitPoints = append(topo.ExitPoints, edge.From)
		}
	}
	sort.Strings(topo.ExitPoints)

	// Find conditional nodes
	for from := range cg.conditionalByFrom {
		topo.ConditionalNodes = append(topo.ConditionalNodes, from)
	}
	sort.Strings(topo.ConditionalNodes)

	// Find isolated nodes (no incoming or outgoing edges)
	for name := range cg.nodes {
		if cg.incoming[name] == 0 && len(cg.outgoing[name]) == 0 {
			topo.IsolatedNodes = append(topo.IsolatedNodes, name)
		}
	}
	sort.Strings(topo.IsolatedNodes)

	// Calculate max depth
	topo.MaxDepth = cg.calculateMaxDepth()

	// Calculate total possible paths
	topo.TotalPaths = cg.estimateTotalPaths()

	return topo
}

// GetMetrics returns runtime metrics about the graph.
func (cg *CompiledGraph) GetMetrics() *GraphMetrics {
	metrics := &GraphMetrics{
		TotalNodes:       len(cg.nodes),
		TotalEdges:       len(cg.edges),
		ConditionalEdges: len(cg.conditionals),
		NodesByType:      make(map[string]int),
	}

	// Calculate fan-out statistics
	totalFanOut := 0
	maxFanOut := 0
	for _, targets := range cg.outgoing {
		fanOut := len(targets)
		totalFanOut += fanOut
		if fanOut > maxFanOut {
			maxFanOut = fanOut
		}
	}
	if len(cg.outgoing) > 0 {
		metrics.AverageFanOut = float64(totalFanOut) / float64(len(cg.outgoing))
	}
	metrics.MaxFanOut = maxFanOut

	// Calculate fan-in statistics
	totalFanIn := 0
	maxFanIn := 0
	for _, fanIn := range cg.incoming {
		totalFanIn += fanIn
		if fanIn > maxFanIn {
			maxFanIn = fanIn
		}
	}
	if len(cg.incoming) > 0 {
		metrics.AverageFanIn = float64(totalFanIn) / float64(len(cg.incoming))
	}
	metrics.MaxFanIn = maxFanIn

	// Calculate cyclomatic complexity: E - N + 2P
	// E = edges, N = nodes, P = connected components (assume 1)
	metrics.CyclomaticComplexity = len(cg.edges) - len(cg.nodes) + 2

	// Count nodes by type
	for _, name := range cg.nodeNames {
		if info, err := cg.GetNodeInfo(name); err == nil {
			metrics.NodesByType[info.Type]++
		}
	}

	// Get runtime state
	cg.runtimeMu.RLock()
	if cg.runtime != nil {
		metrics.CurrentSuperstep = cg.runtime.currentSuperstep()
		metrics.CompletedNodes = cg.runtime.completedNames()
		metrics.PausedNodes = cg.runtime.pausedNames()
	}
	cg.runtimeMu.RUnlock()

	return metrics
}

// GetDependencies returns dependency information for a specific node.
func (cg *CompiledGraph) GetDependencies(name string) (*NodeDependencies, error) {
	if _, exists := cg.nodes[name]; !exists {
		return nil, ErrNodeNotFound
	}

	deps := &NodeDependencies{
		Node:               name,
		DirectPredecessors: make([]string, 0),
		DirectSuccessors:   append([]string(nil), cg.outgoing[name]...),
		AllPredecessors:    make([]string, 0),
		AllSuccessors:      make([]string, 0),
	}

	// Find direct predecessors
	for _, edge := range cg.edges {
		if edge.To == name && edge.From != StartNode {
			deps.DirectPredecessors = append(deps.DirectPredecessors, edge.From)
		}
	}
	sort.Strings(deps.DirectPredecessors)
	sort.Strings(deps.DirectSuccessors)

	// Find all predecessors (transitive closure)
	deps.AllPredecessors = cg.findAllPredecessors(name)
	sort.Strings(deps.AllPredecessors)

	// Find all successors (transitive closure)
	deps.AllSuccessors = cg.findAllSuccessors(name)
	sort.Strings(deps.AllSuccessors)

	// Calculate depth from START
	deps.Depth = cg.calculateDepth(name)

	return deps, nil
}

// GetExecutionPath returns all possible execution paths from START to END.
// Note: This can be expensive for graphs with many conditional branches.
func (cg *CompiledGraph) GetExecutionPath(maxPaths int) [][]string {
	if maxPaths <= 0 {
		maxPaths = 100 // Default limit
	}

	paths := make([][]string, 0)
	currentPath := []string{StartNode}

	cg.findPaths(StartNode, currentPath, &paths, maxPaths)

	return paths
}

// GenerateMermaidFlowchart creates a Mermaid flowchart representation of the graph.
// The direction parameter controls layout: "TD" (top-down), "LR" (left-right),
// "BT" (bottom-top), "RL" (right-left). Default is "TD".
func (cg *CompiledGraph) GenerateMermaidFlowchart(direction string) string {
	if direction == "" {
		direction = "TD"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "flowchart %s\n", strings.ToUpper(direction))

	// Collect and sort all nodes
	nodeNames := cg.collectAllNodes()

	// Generate sanitized node IDs
	idMap := cg.generateNodeIDs(nodeNames)

	// Render nodes with appropriate shapes
	cg.renderNodes(&builder, nodeNames, idMap)

	// Render edges
	cg.renderDirectEdges(&builder, idMap)
	cg.renderConditionalEdges(&builder, idMap)

	return builder.String()
}

func (cg *CompiledGraph) collectAllNodes() []string {
	allNodes := make(map[string]bool)

	for name := range cg.nodes {
		allNodes[name] = true
	}
	for _, edge := range cg.edges {
		if edge.From != "" {
			allNodes[edge.From] = true
		}
		if edge.To != "" {
			allNodes[edge.To] = true
		}
	}
	for _, ce := range cg.conditionals {
		if ce.From != "" {
			allNodes[ce.From] = true
		}
		for _, target := range ce.Targets {
			if target != "" {
				allNodes[target] = true
			}
		}
	}

	nodeNames := make([]string, 0, len(allNodes))
	for name := range allNodes {
		nodeNames = append(nodeNames, name)
	}
	sort.Strings(nodeNames)

	return nodeNames
}

func (cg *CompiledGraph) generateNodeIDs(nodeNames []string) map[string]string {
	reserved := make(map[string]struct{})
	idMap := make(map[string]string)

	for _, name := range nodeNames {
		id := sanitizeMermaidIDForGraph(name, reserved)
		idMap[name] = id
	}

	return idMap
}

func (cg *CompiledGraph) renderNodes(builder *strings.Builder, nodeNames []string, idMap map[string]string) {
	for _, name := range nodeNames {
		id := idMap[name]
		label := escapeMermaidLabel(name)

		var shape string
		switch {
		case name == StartNode || name == EndNode:
			shape = fmt.Sprintf("    %s([%s])\n", id, label) // Stadium shape for START/END
		case cg.conditionalByFrom[name] != nil:
			shape = fmt.Sprintf("    %s{%s}\n", id, label) // Diamond for conditional
		default:
			shape = fmt.Sprintf("    %s[%s]\n", id, label) // Rectangle for standard
		}
		builder.WriteString(shape)
	}
}

func (cg *CompiledGraph) renderDirectEdges(builder *strings.Builder, idMap map[string]string) {
	seenEdges := make(map[string]bool)

	for _, edge := range cg.edges {
		fromID, okFrom := idMap[edge.From]
		toID, okTo := idMap[edge.To]
		if !okFrom || !okTo {
			continue
		}

		edgeKey := fromID + "->" + toID
		if seenEdges[edgeKey] {
			continue
		}
		seenEdges[edgeKey] = true

		fmt.Fprintf(builder, "    %s --> %s\n", fromID, toID)
	}
}

func (cg *CompiledGraph) renderConditionalEdges(builder *strings.Builder, idMap map[string]string) {
	seenConditional := make(map[string]bool)

	for _, ce := range cg.conditionals {
		fromID, okFrom := idMap[ce.From]
		if !okFrom {
			continue
		}

		for _, target := range ce.Targets {
			toID, okTo := idMap[target]
			if !okTo {
				continue
			}

			edgeKey := fromID + "-.>" + toID
			if seenConditional[edgeKey] {
				continue
			}
			seenConditional[edgeKey] = true

			label := escapeMermaidLabel(target)
			fmt.Fprintf(builder, "    %s -.->|%s| %s\n", fromID, label, toID)
		}
	}
}

// Helper methods

func (cg *CompiledGraph) calculateMaxDepth() int {
	depths := make(map[string]int)
	depths[StartNode] = 0

	// BFS to calculate depths
	queue := []string{StartNode}
	maxDepth := 0

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentDepth := depths[current]

		if currentDepth > maxDepth {
			maxDepth = currentDepth
		}

		for _, next := range cg.outgoing[current] {
			if next == EndNode {
				continue
			}
			if _, visited := depths[next]; !visited {
				depths[next] = currentDepth + 1
				queue = append(queue, next)
			}
		}
	}

	return maxDepth
}

func (cg *CompiledGraph) calculateDepth(name string) int {
	if name == StartNode {
		return 0
	}

	depths := make(map[string]int)
	depths[StartNode] = 0

	queue := []string{StartNode}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentDepth := depths[current]

		for _, next := range cg.outgoing[current] {
			if next == EndNode {
				continue
			}
			if _, visited := depths[next]; !visited {
				depths[next] = currentDepth + 1
				if next == name {
					return currentDepth + 1
				}
				queue = append(queue, next)
			}
		}
	}

	return -1 // Not reachable from START
}

func (cg *CompiledGraph) estimateTotalPaths() int {
	// Simple estimation: multiply branching factors
	// For accurate count, would need full path enumeration (expensive)
	paths := 1
	for _, conditionals := range cg.conditionalByFrom {
		for _, ce := range conditionals {
			if len(ce.Targets) > 1 {
				paths *= len(ce.Targets)
			}
		}
	}
	return paths
}

func (cg *CompiledGraph) findAllPredecessors(name string) []string {
	visited := make(map[string]bool)
	result := make([]string, 0)

	var dfs func(string)
	dfs = func(node string) {
		if node == StartNode || visited[node] {
			return
		}
		visited[node] = true
		result = append(result, node)

		// Find all predecessors
		for _, edge := range cg.edges {
			if edge.To == node {
				dfs(edge.From)
			}
		}
	}

	dfs(name)
	return result
}

func (cg *CompiledGraph) findAllSuccessors(name string) []string {
	visited := make(map[string]bool)
	result := make([]string, 0)

	var dfs func(string)
	dfs = func(node string) {
		if node == EndNode || visited[node] {
			return
		}
		visited[node] = true
		result = append(result, node)

		// Find all successors
		for _, next := range cg.outgoing[node] {
			dfs(next)
		}
	}

	// Start from direct successors
	for _, next := range cg.outgoing[name] {
		dfs(next)
	}

	return result
}

func (cg *CompiledGraph) findPaths(current string, path []string, paths *[][]string, maxPaths int) {
	if len(*paths) >= maxPaths {
		return
	}

	if current == EndNode {
		// Found complete path
		completePath := make([]string, len(path))
		copy(completePath, path)
		*paths = append(*paths, completePath)
		return
	}

	// Explore outgoing edges
	for _, next := range cg.outgoing[current] {
		// Check for cycles
		hasCycle := false
		for _, p := range path {
			if p == next {
				hasCycle = true
				break
			}
		}
		if !hasCycle {
			path = append(path, next)
			cg.findPaths(next, path, paths, maxPaths)
		}
	}

	// Explore conditional edges
	if conditionals, exists := cg.conditionalByFrom[current]; exists {
		for _, ce := range conditionals {
			for _, target := range ce.Targets {
				// Check for cycles
				hasCycle := false
				for _, p := range path {
					if p == target {
						hasCycle = true
						break
					}
				}
				if !hasCycle {
					path = append(path, target)
					cg.findPaths(target, path, paths, maxPaths)
				}
			}
		}
	}
}

// escapeMermaidLabel escapes special characters for Mermaid labels
func escapeMermaidLabel(value string) string {
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "<br/>")
	return value
}

// sanitizeMermaidID creates a valid Mermaid node ID from a name
func sanitizeMermaidIDForGraph(name string, reserved map[string]struct{}) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "node"
	}

	var builder strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune('_')
		case unicode.IsSpace(r):
			builder.WriteRune('_')
		default:
			builder.WriteRune('_')
		}
	}

	id := builder.String()
	if id == "" {
		id = "node"
	}
	if r := rune(id[0]); !unicode.IsLetter(r) && r != '_' {
		id = "n_" + id
	}

	candidate := id
	counter := 1
	for {
		if _, exists := reserved[candidate]; !exists {
			reserved[candidate] = struct{}{}
			return candidate
		}
		counter++
		candidate = fmt.Sprintf("%s_%d", id, counter)
	}
}
