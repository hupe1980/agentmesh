package compile

import (
	"fmt"
	"sort"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// GetNodes returns a sorted list of all node names in the compiled graph.
func (cg *CompiledGraph) GetNodes() []string {
	return cg.Topology.NodeNames
}

// GetNodeInfo returns detailed information about a specific node.
// This uses the computed topology for accurate edge counts.
func (cg *CompiledGraph) GetNodeInfo(name string) (*NodeInfo, error) {
	node, exists := cg.Graph.Nodes[name]
	if !exists {
		return nil, fmt.Errorf("node not found: %s", name)
	}

	// Check if node supports retry policy
	var retryPolicy *graph.RetryPolicy
	if retryNode, ok := node.(graph.NodeWithRetry); ok {
		retryPolicy = retryNode.RetryPolicy()
	}

	info := &NodeInfo{
		Name:              name,
		Type:              "standard",
		IncomingEdges:     cg.Topology.Incoming[name],
		OutgoingEdges:     len(cg.Topology.Outgoing[name]),
		IsConditional:     len(cg.Topology.ConditionalByFrom[name]) > 0,
		IsConditionalGate: cg.Topology.ConditionalGate[name],
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

// GetAllNodeInfo returns information about all nodes in the compiled graph.
func (cg *CompiledGraph) GetAllNodeInfo() []NodeInfo {
	result := make([]NodeInfo, 0, len(cg.Graph.Nodes))
	for _, name := range cg.Topology.NodeNames {
		if info, err := cg.GetNodeInfo(name); err == nil {
			result = append(result, *info)
		}
	}
	return result
}

// GetEdges returns information about all edges in the compiled graph.
// This uses deduplicated edges from the topology.
func (cg *CompiledGraph) GetEdges() []EdgeInfo {
	edges := make([]EdgeInfo, 0)

	// Direct edges (deduplicated)
	for _, edge := range cg.Topology.Edges {
		edges = append(edges, EdgeInfo{
			From: edge.From,
			To:   edge.To,
			Type: "direct",
		})
	}

	// Conditional edges
	for from, conditionals := range cg.Topology.ConditionalByFrom {
		for _, ce := range conditionals {
			targets := make([]string, len(ce.Targets))
			copy(targets, ce.Targets)
			sort.Strings(targets)

			edges = append(edges, EdgeInfo{
				From:               from,
				To:                 "",
				Type:               "conditional",
				ConditionalTargets: targets,
			})
		}
	}

	return edges
}

// GetTopology returns a comprehensive view of the compiled graph structure.
func (cg *CompiledGraph) GetTopology() *Topology {
	topo := &Topology{
		Nodes:            cg.GetAllNodeInfo(),
		Edges:            cg.GetEdges(),
		EntryPoints:      make([]string, 0),
		ExitPoints:       make([]string, 0),
		ConditionalNodes: make([]string, 0),
		IsolatedNodes:    make([]string, 0),
	}

	// Find entry points (nodes with edges from START)
	if targets, ok := cg.Topology.Outgoing[StartNode]; ok {
		topo.EntryPoints = append(topo.EntryPoints, targets...)
		sort.Strings(topo.EntryPoints)
	}

	// Find exit points (nodes with edges to END)
	for _, edge := range cg.Topology.Edges {
		if edge.To == EndNode && edge.From != StartNode {
			topo.ExitPoints = append(topo.ExitPoints, edge.From)
		}
	}
	sort.Strings(topo.ExitPoints)

	// Find conditional nodes
	for from := range cg.Topology.ConditionalByFrom {
		topo.ConditionalNodes = append(topo.ConditionalNodes, from)
	}
	sort.Strings(topo.ConditionalNodes)

	// Find isolated nodes (no incoming or outgoing edges)
	for name := range cg.Graph.Nodes {
		if cg.Topology.Incoming[name] == 0 && len(cg.Topology.Outgoing[name]) == 0 {
			topo.IsolatedNodes = append(topo.IsolatedNodes, name)
		}
	}
	sort.Strings(topo.IsolatedNodes)

	// Calculate max depth
	topo.MaxDepth = cg.calculateMaxDepth()

	// Calculate total possible paths (estimate)
	topo.TotalPaths = cg.estimateTotalPaths()

	return topo
}

// GetMetrics returns runtime metrics about the compiled graph.
func (cg *CompiledGraph) GetMetrics() *Metrics {
	metrics := &Metrics{
		TotalNodes:       len(cg.Graph.Nodes),
		TotalEdges:       len(cg.Topology.Edges),
		ConditionalEdges: len(cg.Topology.ConditionalByFrom),
		NodesByType:      make(map[string]int),
	}

	// Calculate fan-out statistics using topology
	totalFanOut := 0
	for _, targets := range cg.Topology.Outgoing {
		fanOut := len(targets)
		totalFanOut += fanOut
		if fanOut > metrics.MaxFanOut {
			metrics.MaxFanOut = fanOut
		}
	}
	if len(cg.Topology.Outgoing) > 0 {
		metrics.AverageFanOut = float64(totalFanOut) / float64(len(cg.Topology.Outgoing))
	}

	// Calculate fan-in statistics using topology
	totalFanIn := 0
	for _, count := range cg.Topology.Incoming {
		if count > metrics.MaxFanIn {
			metrics.MaxFanIn = count
		}
		totalFanIn += count
	}
	if len(cg.Topology.Incoming) > 0 {
		metrics.AverageFanIn = float64(totalFanIn) / float64(len(cg.Topology.Incoming))
	}

	// Count nodes by type
	for _, name := range cg.Topology.NodeNames {
		info, _ := cg.GetNodeInfo(name)
		if info != nil {
			metrics.NodesByType[info.Type]++
		}
	}

	// Calculate cyclomatic complexity: E - N + 2P
	// E = edges, N = nodes, P = connected components (assume 1)
	metrics.CyclomaticComplexity = len(cg.Topology.Edges) - len(cg.Graph.Nodes) + 2

	return metrics
}

// GetNodeDependencies returns dependency information for a specific node.
func (cg *CompiledGraph) GetNodeDependencies(name string) (*NodeDependencies, error) {
	if _, exists := cg.Graph.Nodes[name]; !exists {
		return nil, fmt.Errorf("node not found: %s", name)
	}

	deps := &NodeDependencies{
		NodeName:           name,
		DirectPredecessors: make([]string, 0),
		DirectSuccessors:   make([]string, 0),
		AllPredecessors:    make([]string, 0),
		AllSuccessors:      make([]string, 0),
	}

	// Find direct predecessors (nodes with edges to this node)
	for _, edge := range cg.Topology.Edges {
		if edge.To == name {
			deps.DirectPredecessors = append(deps.DirectPredecessors, edge.From)
		}
	}
	sort.Strings(deps.DirectPredecessors)

	// Find direct successors from topology
	if targets, ok := cg.Topology.Outgoing[name]; ok {
		deps.DirectSuccessors = append(deps.DirectSuccessors, targets...)
		sort.Strings(deps.DirectSuccessors)
	}

	// Find all predecessors (recursive)
	deps.AllPredecessors = cg.findAllPredecessors(name)
	sort.Strings(deps.AllPredecessors)

	// Find all successors (recursive)
	deps.AllSuccessors = cg.findAllSuccessors(name)
	sort.Strings(deps.AllSuccessors)

	// Calculate depth from START
	deps.Depth = cg.calculateDepth(name)

	return deps, nil
}

// MermaidFlowchart generates a Mermaid flowchart representation of the compiled graph.
// This uses the deduplicated topology for accuracy.
func (cg *CompiledGraph) MermaidFlowchart(direction string) string {
	if direction == "" {
		direction = "TD"
	}

	var result string
	result += fmt.Sprintf("graph %s\n", direction)

	// Add nodes
	for _, nodeName := range cg.Topology.NodeNames {
		// Style based on node type
		switch nodeName {
		case StartNode:
			result += fmt.Sprintf("    %s([START])\n", nodeName)
		case EndNode:
			result += fmt.Sprintf("    %s([END])\n", nodeName)
		default:
			// Check if conditional
			if len(cg.Topology.ConditionalByFrom[nodeName]) > 0 {
				result += fmt.Sprintf("    %s{%s}\n", nodeName, nodeName)
			} else {
				result += fmt.Sprintf("    %s[%s]\n", nodeName, nodeName)
			}
		}
	}

	// Add edges
	for _, edge := range cg.Topology.Edges {
		result += fmt.Sprintf("    %s --> %s\n", edge.From, edge.To)
	}

	// Add conditional edges
	for from, conditionals := range cg.Topology.ConditionalByFrom {
		for _, ce := range conditionals {
			for _, target := range ce.Targets {
				result += fmt.Sprintf("    %s -.-> %s\n", from, target)
			}
		}
	}

	return result
}

// Helper functions

func (cg *CompiledGraph) calculateMaxDepth() int {
	maxDepth := 0
	for _, name := range cg.Topology.NodeNames {
		depth := cg.calculateDepth(name)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

func (cg *CompiledGraph) calculateDepth(name string) int {
	if name == StartNode {
		return 0
	}

	visited := make(map[string]bool)
	return cg.calculateDepthRecursive(name, visited)
}

func (cg *CompiledGraph) calculateDepthRecursive(name string, visited map[string]bool) int {
	if name == StartNode {
		return 0
	}

	if visited[name] {
		return 0 // Cycle detected, stop
	}
	visited[name] = true

	maxPredDepth := -1
	for _, edge := range cg.Topology.Edges {
		if edge.To == name {
			predDepth := cg.calculateDepthRecursive(edge.From, visited)
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

func (cg *CompiledGraph) estimateTotalPaths() int {
	// Simple estimation: count branches
	paths := 1
	for _, conditionals := range cg.Topology.ConditionalByFrom {
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
	cg.findAllPredecessorsRecursive(name, visited, &result)
	return result
}

func (cg *CompiledGraph) findAllPredecessorsRecursive(name string, visited map[string]bool, result *[]string) {
	if visited[name] {
		return
	}
	visited[name] = true

	for _, edge := range cg.Topology.Edges {
		if edge.To == name && edge.From != StartNode {
			*result = append(*result, edge.From)
			cg.findAllPredecessorsRecursive(edge.From, visited, result)
		}
	}
}

func (cg *CompiledGraph) findAllSuccessors(name string) []string {
	visited := make(map[string]bool)
	result := make([]string, 0)
	cg.findAllSuccessorsRecursive(name, visited, &result)
	return result
}

func (cg *CompiledGraph) findAllSuccessorsRecursive(name string, visited map[string]bool, result *[]string) {
	if visited[name] {
		return
	}
	visited[name] = true

	if targets, ok := cg.Topology.Outgoing[name]; ok {
		for _, target := range targets {
			if target != EndNode {
				*result = append(*result, target)
				cg.findAllSuccessorsRecursive(target, visited, result)
			}
		}
	}
}
