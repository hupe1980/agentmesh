package compile

import (
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ValidationError represents a graph validation error with context.
type ValidationError struct {
	Type    ValidationErrorType
	Message string
	Node    string // Node name if applicable
	Details map[string]any
}

func (e *ValidationError) Error() string {
	if e.Node != "" {
		return fmt.Sprintf("%s: node %q: %s", e.Type, e.Node, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// ValidationErrorType categorizes validation errors.
type ValidationErrorType string

const (
	ErrTypeMissingNode      ValidationErrorType = "missing_node"
	ErrTypeDuplicateNode    ValidationErrorType = "duplicate_node"
	ErrTypeInvalidNode      ValidationErrorType = "invalid_node"
	ErrTypeCycle            ValidationErrorType = "cycle_detected"
	ErrTypeUnreachableNode  ValidationErrorType = "unreachable_node"
	ErrTypeDeadEnd          ValidationErrorType = "dead_end_node"
	ErrTypeMissingStart     ValidationErrorType = "missing_start"
	ErrTypeMissingEnd       ValidationErrorType = "missing_end"
	ErrTypeInvalidEdge      ValidationErrorType = "invalid_edge"
	ErrTypeEmptyGraph       ValidationErrorType = "empty_graph"
	ErrTypeInvalidCondition ValidationErrorType = "invalid_condition"
)

// ValidationOptions controls which validations are performed.
type ValidationOptions struct {
	// SkipValidation disables all validation (use with caution)
	SkipValidation bool

	// StrictMode enables all validations including warnings
	StrictMode bool

	// AllowUnreachable permits nodes not reachable from START
	AllowUnreachable bool

	// AllowDeadEnds permits nodes with no path to END
	AllowDeadEnds bool

	// AllowCycles permits cycles in the graph (for iterative algorithms)
	AllowCycles bool

	// RequireStartNode ensures at least one node is connected to START
	RequireStartNode bool

	// RequireEndNode ensures at least one node is connected to END
	RequireEndNode bool
}

// DefaultValidationOptions returns validation options suitable for most use cases.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		StrictMode:       false,
		AllowUnreachable: true,  // Unreachable nodes simply won't execute
		AllowDeadEnds:    true,  // Dead ends are valid (e.g., logging nodes)
		AllowCycles:      true,  // Cycles needed for iterative patterns
		RequireStartNode: false, // Empty graphs are valid
		RequireEndNode:   false,
	}
}

// StrictValidationOptions returns strict validation for production graphs.
func StrictValidationOptions() ValidationOptions {
	return ValidationOptions{
		StrictMode:       true,
		AllowUnreachable: false,
		AllowDeadEnds:    false,
		AllowCycles:      false, // Usually a bug
		RequireStartNode: true,
		RequireEndNode:   true,
	}
}

// Validator validates graph structure before compilation.
type Validator struct {
	opts ValidationOptions
}

// NewValidator creates a new validator with the given options.
func NewValidator(opts ValidationOptions) *Validator {
	return &Validator{opts: opts}
}

// Validate performs comprehensive validation of a graph.
func (v *Validator) Validate(g *graph.Graph) []ValidationError {
	// Skip all validation if requested
	if v.opts.SkipValidation {
		return nil
	}

	var errors []ValidationError

	// Basic structure validation
	errors = append(errors, v.validateBasicStructure(g)...)
	if len(errors) > 0 && !v.opts.StrictMode {
		// Stop early if basic validation fails and not in strict mode
		return errors
	}

	// Edge validation
	errors = append(errors, v.validateEdges(g)...)

	// Conditional validation
	errors = append(errors, v.validateConditionals(g)...)

	// Topology validation (requires valid edges)
	if !hasBlockingErrors(errors) {
		errors = append(errors, v.validateTopology(g)...)
	}

	return errors
}

// validateBasicStructure checks basic graph structure.
func (v *Validator) validateBasicStructure(g *graph.Graph) []ValidationError {
	var errors []ValidationError

	// Empty graph check
	if len(g.Nodes) == 0 {
		if v.opts.RequireStartNode || v.opts.StrictMode {
			errors = append(errors, ValidationError{
				Type:    ErrTypeEmptyGraph,
				Message: "graph has no nodes",
			})
		}
		return errors
	}

	// Check for nil or invalid nodes
	for name, node := range g.Nodes {
		if node == nil {
			errors = append(errors, ValidationError{
				Type:    ErrTypeInvalidNode,
				Node:    name,
				Message: "node is nil",
			})
			continue
		}

		nodeName := node.Name()

		if nodeName == "" {
			errors = append(errors, ValidationError{
				Type:    ErrTypeInvalidNode,
				Node:    name,
				Message: "node has empty name",
			})
		}

		if nodeName != name {
			errors = append(errors, ValidationError{
				Type:    ErrTypeDuplicateNode,
				Node:    name,
				Message: fmt.Sprintf("node name mismatch: map key %q vs node.Name() %q", name, nodeName),
			})
		}

		// Node interface guarantees Execute method exists, no need to check for RunFunc
	}

	// Check for reserved node names
	for name := range g.Nodes {
		if name == StartNode || name == EndNode {
			errors = append(errors, ValidationError{
				Type:    ErrTypeInvalidNode,
				Node:    name,
				Message: fmt.Sprintf("node uses reserved name %q", name),
			})
		}
	}

	return errors
}

// validateEdges checks all edges reference existing nodes.
func (v *Validator) validateEdges(g *graph.Graph) []ValidationError {
	var errors []ValidationError

	// Track nodes with connections to/from START/END
	hasStartConnection := false
	hasEndConnection := false

	for _, edge := range g.Edges {
		// Validate FROM node
		if edge.From != StartNode && edge.From != EndNode {
			if _, exists := g.Nodes[edge.From]; !exists {
				errors = append(errors, ValidationError{
					Type:    ErrTypeMissingNode,
					Message: fmt.Sprintf("edge references non-existent source node %q", edge.From),
					Details: map[string]any{"edge": fmt.Sprintf("%s -> %s", edge.From, edge.To)},
				})
			}
		}

		// Validate TO node
		if edge.To != StartNode && edge.To != EndNode {
			if _, exists := g.Nodes[edge.To]; !exists {
				errors = append(errors, ValidationError{
					Type:    ErrTypeMissingNode,
					Message: fmt.Sprintf("edge references non-existent target node %q", edge.To),
					Details: map[string]any{"edge": fmt.Sprintf("%s -> %s", edge.From, edge.To)},
				})
			}
		}

		// Track START/END connections
		if edge.From == StartNode && edge.To != EndNode {
			hasStartConnection = true
		}
		if edge.To == EndNode && edge.From != StartNode {
			hasEndConnection = true
		}

		// Check for invalid edge patterns
		if edge.From == EndNode {
			errors = append(errors, ValidationError{
				Type:    ErrTypeInvalidEdge,
				Message: fmt.Sprintf("edge from END node is invalid: %s -> %s", edge.From, edge.To),
			})
		}
		if edge.To == StartNode {
			errors = append(errors, ValidationError{
				Type:    ErrTypeInvalidEdge,
				Message: fmt.Sprintf("edge to START node is invalid: %s -> %s", edge.From, edge.To),
			})
		}
	}

	// Check START connection requirement
	if v.opts.RequireStartNode && !hasStartConnection {
		errors = append(errors, ValidationError{
			Type:    ErrTypeMissingStart,
			Message: "no nodes are connected from START",
		})
	}

	// Check END connection requirement
	if v.opts.RequireEndNode && !hasEndConnection {
		errors = append(errors, ValidationError{
			Type:    ErrTypeMissingEnd,
			Message: "no nodes are connected to END",
		})
	}

	return errors
}

// validateConditionals checks conditional edges.
func (v *Validator) validateConditionals(g *graph.Graph) []ValidationError {
	var errors []ValidationError

	for _, cond := range g.Branches {
		// Check FROM node exists
		if _, exists := g.Nodes[cond.From]; !exists {
			errors = append(errors, ValidationError{
				Type:    ErrTypeMissingNode,
				Node:    cond.From,
				Message: "conditional edge references non-existent source node",
			})
		}

		// Check condition function
		if cond.Condition == nil {
			errors = append(errors, ValidationError{
				Type:    ErrTypeInvalidCondition,
				Node:    cond.From,
				Message: "conditional edge has nil condition function",
			})
		}

		// Check all target nodes exist
		for _, target := range cond.Targets {
			if target == StartNode {
				errors = append(errors, ValidationError{
					Type:    ErrTypeInvalidEdge,
					Node:    cond.From,
					Message: "conditional edge cannot target START node",
					Details: map[string]any{"target": target},
				})
			}
			if target != EndNode {
				if _, exists := g.Nodes[target]; !exists {
					errors = append(errors, ValidationError{
						Type:    ErrTypeMissingNode,
						Node:    cond.From,
						Message: fmt.Sprintf("conditional edge references non-existent target node %q", target),
					})
				}
			}
		}
	}

	return errors
}

// validateTopology checks graph topology (cycles, reachability, etc.).
func (v *Validator) validateTopology(g *graph.Graph) []ValidationError {
	var errors []ValidationError

	// Build adjacency list
	adj := make(map[string][]string)
	for _, edge := range g.Edges {
		// Skip START/END for topology analysis
		if edge.From == StartNode || edge.To == EndNode {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	// Add conditional edges to adjacency list
	for _, cond := range g.Branches {
		for _, target := range cond.Targets {
			if target != EndNode {
				adj[cond.From] = append(adj[cond.From], target)
			}
		}
	}

	// Check for cycles
	if !v.opts.AllowCycles {
		cycles := v.findCycles(g, adj)
		for _, cycle := range cycles {
			errors = append(errors, ValidationError{
				Type:    ErrTypeCycle,
				Message: fmt.Sprintf("cycle detected: %s", strings.Join(cycle, " -> ")),
				Details: map[string]any{"cycle": cycle},
			})
		}
	}

	// Check for unreachable nodes
	if !v.opts.AllowUnreachable {
		unreachable := v.findUnreachableNodes(g, adj)
		for _, node := range unreachable {
			errors = append(errors, ValidationError{
				Type:    ErrTypeUnreachableNode,
				Node:    node,
				Message: "node is not reachable from START",
			})
		}
	}

	// Check for dead end nodes
	if !v.opts.AllowDeadEnds {
		deadEnds := v.findDeadEndNodes(g, adj)
		for _, node := range deadEnds {
			errors = append(errors, ValidationError{
				Type:    ErrTypeDeadEnd,
				Node:    node,
				Message: "node has no path to END",
			})
		}
	}

	return errors
}

// findCycles detects cycles using DFS.
func (v *Validator) findCycles(g *graph.Graph, adj map[string][]string) [][]string {
	var cycles [][]string
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	path := []string{}

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, neighbor := range adj[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				// Found cycle - extract it from path
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path)-cycleStart)
					copy(cycle, path[cycleStart:])
					cycle = append(cycle, neighbor) // Close the cycle
					cycles = append(cycles, cycle)
				}
				return true
			}
		}

		path = path[:len(path)-1]
		recStack[node] = false
		return false
	}

	for name := range g.Nodes {
		if !visited[name] {
			path = []string{}
			dfs(name)
		}
	}

	return cycles
}

// findUnreachableNodes finds nodes not reachable from START.
func (v *Validator) findUnreachableNodes(g *graph.Graph, adj map[string][]string) []string {
	// Find nodes reachable from START
	reachable := make(map[string]bool)
	queue := []string{}

	// Start with nodes directly connected to START
	for _, edge := range g.Edges {
		if edge.From == StartNode && edge.To != EndNode {
			queue = append(queue, edge.To)
			reachable[edge.To] = true
		}
	}

	// BFS from START-connected nodes
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, neighbor := range adj[current] {
			if !reachable[neighbor] {
				reachable[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	// Find unreachable nodes
	var unreachable []string
	for name := range g.Nodes {
		if !reachable[name] {
			unreachable = append(unreachable, name)
		}
	}

	return unreachable
}

// findDeadEndNodes finds nodes with no path to END.
func (v *Validator) findDeadEndNodes(g *graph.Graph, adj map[string][]string) []string {
	// Build reverse adjacency list
	reverseAdj := make(map[string][]string)
	for from, targets := range adj {
		for _, to := range targets {
			reverseAdj[to] = append(reverseAdj[to], from)
		}
	}

	// Add reverse edges from nodes to END
	for _, edge := range g.Edges {
		if edge.To == EndNode && edge.From != StartNode {
			reverseAdj[EndNode] = append(reverseAdj[EndNode], edge.From)
		}
	}

	// Find nodes reachable from END (backwards)
	reachable := make(map[string]bool)
	queue := []string{EndNode}
	reachable[EndNode] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, neighbor := range reverseAdj[current] {
			if !reachable[neighbor] {
				reachable[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	// Find nodes not reachable from END
	var deadEnds []string
	for name := range g.Nodes {
		if !reachable[name] {
			deadEnds = append(deadEnds, name)
		}
	}

	return deadEnds
}

// hasBlockingErrors checks if any errors prevent further validation.
func hasBlockingErrors(errors []ValidationError) bool {
	for _, err := range errors {
		if err.Type == ErrTypeEmptyGraph || err.Type == ErrTypeMissingNode {
			return true
		}
	}
	return false
}
