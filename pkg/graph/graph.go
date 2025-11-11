package graph

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
)

const (
	// StartNode is the reserved node name for graph entry points.
	StartNode = "__start__"
	// EndNode is the reserved node name for graph exit points.
	EndNode = "__end__"
)

// Graph represents a computational graph with nodes, edges, and conditional branches.
type Graph struct {
	Nodes        map[string]*Node
	Edges        []Edge
	Branches     []ConditionalEdges
	stateManager StateManager
	executor     Executor
	runtime      *executionState

	mu       sync.Mutex
	compiled bool
}

func ensureExecutionState(rt *executionState) *executionState {
	if rt != nil {
		return rt
	}
	return newExecutionState()
}

// NewGraph creates a new graph with the given state manager.
// If stateManager is nil, creates a default State with unlimited messages.
func NewGraph(stateManager StateManager) *Graph {
	if stateManager == nil {
		stateManager = NewStateManager(0) // Unlimited messages by default
	}
	return &Graph{
		Nodes:        make(map[string]*Node),
		Edges:        make([]Edge, 0),
		Branches:     make([]ConditionalEdges, 0),
		stateManager: stateManager,
		runtime:      newExecutionState(),
	}
}

// StateManager returns the graph's state manager.
func (g *Graph) StateManager() StateManager {
	return g.stateManager
}

// AddNode registers a node in the graph.
func (g *Graph) AddNode(n *Node) error {
	if n == nil {
		return ErrNilNode
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if n.Name == "" {
		return &ValidationError{
			Field:   "Name",
			Message: "node name must not be empty",
		}
	}
	if n.RunFunc == nil {
		return ErrNilRunFunc
	}
	if g.Nodes == nil {
		g.Nodes = make(map[string]*Node)
	}
	if _, exists := g.Nodes[n.Name]; exists {
		return &ValidationError{
			Field:   "Name",
			Value:   n.Name,
			Message: "node already exists",
		}
	}
	g.Nodes[n.Name] = n
	return nil
}

// AddEdge creates a directed edge from one node to another.
func (g *Graph) AddEdge(from, to string) {
	g.mu.Lock()
	g.Edges = append(g.Edges, Edge{From: from, To: to})
	g.mu.Unlock()
}

// AddConditionalEdges creates dynamic edges based on runtime conditions.
func (g *Graph) AddConditionalEdges(from string, condition func(context.Context, StateReader) []string, targets []string) {
	if len(targets) == 0 {
		return
	}
	copyTargets := cloneStringSlice(targets)
	g.mu.Lock()
	g.Branches = append(g.Branches, ConditionalEdges{From: from, Targets: copyTargets, Condition: condition})
	g.mu.Unlock()
}

// Compile validates and prepares the graph for execution.
func (g *Graph) Compile() (*Compiled, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}

	g.mu.Lock()
	g.compiled = true
	g.runtime = ensureExecutionState(g.runtime)
	runtime := g.runtime
	stateManager := g.stateManager

	nodes := make(map[string]*Node, len(g.Nodes))
	maps.Copy(nodes, g.Nodes)

	edges := append([]Edge(nil), g.Edges...)

	conditionals := make([]ConditionalEdges, 0, len(g.Branches))
	for _, branch := range g.Branches {
		conditionals = append(conditionals, ConditionalEdges{
			From:      branch.From,
			Targets:   cloneStringSlice(branch.Targets),
			Condition: branch.Condition,
		})
	}
	g.mu.Unlock()

	topo := computeTopology(nodes, edges, conditionals)

	outgoing := make(map[string][]string, len(topo.outgoing))
	for from, targets := range topo.outgoing {
		outgoing[from] = append([]string(nil), targets...)
	}

	conditionalByFrom := make(map[string][]ConditionalEdges, len(topo.conditionalByFrom))
	for from, edges := range topo.conditionalByFrom {
		copyEdges := make([]ConditionalEdges, 0, len(edges))
		for _, edge := range edges {
			copyEdges = append(copyEdges, ConditionalEdges{
				From:      edge.From,
				Targets:   cloneStringSlice(edge.Targets),
				Condition: edge.Condition,
			})
		}
		conditionalByFrom[from] = copyEdges
	}

	cg := &Compiled{
		stateManager:      stateManager,
		executor:          g.executor, // Use custom executor if set, otherwise defaults to Pregel
		runtime:           runtime,
		nodes:             nodes,
		edges:             topo.edges,
		conditionals:      conditionals,
		incoming:          topo.incoming,
		conditionalGate:   topo.conditionalGate,
		outgoing:          outgoing,
		conditionalByFrom: conditionalByFrom,
		nodeNames:         topo.nodeNames,
		startKey:          StartNode,
		endKey:            EndNode,
	}

	return cg, nil
}

// MustCompile compiles the graph into an immutable executable form.
// Panics if validation fails. Use this in tests or when you're certain the graph is valid.
func (g *Graph) MustCompile() *Compiled {
	cg, err := g.Compile()
	if err != nil {
		panic(fmt.Errorf("graph compilation failed: %w", err))
	}
	return cg
}

// Validate checks the graph for structural errors.
//
//nolint:gocyclo // Graph validation requires checking many conditions
func (g *Graph) Validate() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.Nodes) == 0 {
		return fmt.Errorf("graph must contain at least one node")
	}

	containsNode := func(name string) bool {
		if name == "" {
			return false
		}
		_, ok := g.Nodes[name]
		return ok
	}

	hasStartEdge := false

	for _, edge := range g.Edges {
		if edge.From == "" || edge.To == "" {
			return fmt.Errorf("edges must not have empty endpoints")
		}
		if edge.From != StartNode && edge.From != EndNode && !containsNode(edge.From) {
			return fmt.Errorf("found edge starting at unknown node `%s`", edge.From)
		}
		if edge.To != EndNode && edge.To != StartNode && !containsNode(edge.To) {
			return fmt.Errorf("found edge ending at unknown node `%s`", edge.To)
		}
		if edge.From == StartNode {
			hasStartEdge = true
		}
	}

	if !hasStartEdge {
		return fmt.Errorf("graph must have an entrypoint: add at least one edge from START to another node")
	}

	for _, ce := range g.Branches {
		if ce.From == "" {
			return fmt.Errorf("conditional edge must declare a source node")
		}
		if ce.From != StartNode && !containsNode(ce.From) {
			return fmt.Errorf("conditional edge starts at unknown node `%s`", ce.From)
		}
		if len(ce.Targets) == 0 {
			return fmt.Errorf("conditional edge from `%s` must declare at least one target", ce.From)
		}
		for _, target := range ce.Targets {
			if target == "" {
				return fmt.Errorf("conditional edge from `%s` references empty target", ce.From)
			}
			if target != EndNode && !containsNode(target) {
				return fmt.Errorf("conditional edge from `%s` references unknown target `%s`", ce.From, target)
			}
		}
	}

	// Check for unreachable nodes (nodes not reachable from START)
	reachable := g.computeReachableNodes()
	for name := range g.Nodes {
		if !reachable[name] {
			return fmt.Errorf("%w: node %q cannot be reached from START", ErrUnreachableNode, name)
		}
	}

	return nil
}

// computeReachableNodes returns the set of nodes reachable from START via BFS
func (g *Graph) computeReachableNodes() map[string]bool {
	reachable := make(map[string]bool)
	queue := []string{StartNode}
	reachable[StartNode] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Follow static edges
		for _, edge := range g.Edges {
			if edge.From == current {
				if !reachable[edge.To] {
					reachable[edge.To] = true
					if edge.To != EndNode {
						queue = append(queue, edge.To)
					}
				}
			}
		}

		// Follow conditional edges
		for _, ce := range g.Branches {
			if ce.From == current {
				for _, target := range ce.Targets {
					if !reachable[target] {
						reachable[target] = true
						if target != EndNode {
							queue = append(queue, target)
						}
					}
				}
			}
		}
	}

	return reachable
}

type graphTopology struct {
	edges             []Edge
	incoming          map[string]int
	outgoing          map[string][]string
	conditionalGate   map[string]bool
	conditionalByFrom map[string][]ConditionalEdges
	nodeNames         []string
}

//nolint:gocyclo // Topology computation requires handling various edge cases
func computeTopology(nodes map[string]*Node, edges []Edge, branches []ConditionalEdges) graphTopology {
	topo := graphTopology{
		incoming:          make(map[string]int, len(nodes)),
		outgoing:          make(map[string][]string),
		conditionalGate:   make(map[string]bool),
		conditionalByFrom: make(map[string][]ConditionalEdges),
	}

	for name := range nodes {
		topo.incoming[name] = 0
	}

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
		topo.edges = append(topo.edges, edge)
		topo.outgoing[edge.From] = append(topo.outgoing[edge.From], edge.To)
		if edge.To != EndNode {
			if _, ok := topo.incoming[edge.To]; !ok {
				topo.incoming[edge.To] = 0
			}
			if edge.From != StartNode {
				topo.incoming[edge.To]++
			}
		}
	}

	for _, branch := range branches {
		if len(branch.Targets) == 0 {
			continue
		}
		copyTargets := cloneStringSlice(branch.Targets)
		topo.conditionalByFrom[branch.From] = append(topo.conditionalByFrom[branch.From], ConditionalEdges{
			From:      branch.From,
			Targets:   copyTargets,
			Condition: branch.Condition,
		})
		for _, target := range copyTargets {
			if target == "" {
				continue
			}
			topo.conditionalGate[target] = true
			if _, ok := topo.incoming[target]; !ok {
				topo.incoming[target] = 0
			}
		}
	}

	if len(topo.conditionalGate) == 0 {
		topo.conditionalGate = nil
	}
	if len(topo.conditionalByFrom) == 0 {
		topo.conditionalByFrom = nil
	}

	topo.nodeNames = make([]string, 0, len(nodes))
	for name := range nodes {
		topo.nodeNames = append(topo.nodeNames, name)
	}
	sort.Strings(topo.nodeNames)

	return topo
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
