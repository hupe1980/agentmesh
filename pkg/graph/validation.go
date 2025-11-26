package graph

import (
	"fmt"
)

// ValidationErrorType classifies validation errors.
type ValidationErrorType string

// Validation error type constants.
const (
	// ErrorTypeCycle indicates a cycle in the graph.
	ErrorTypeCycle            ValidationErrorType = "CYCLE"
	ErrorTypeDisconnected     ValidationErrorType = "DISCONNECTED"
	ErrorTypeInvalidEntryNode ValidationErrorType = "INVALID_ENTRY_NODE"
	ErrorTypeInvalidEndNode   ValidationErrorType = "INVALID_END_NODE"
	ErrorTypeMissingNode      ValidationErrorType = "MISSING_NODE"
	ErrorTypeInvalidBranch    ValidationErrorType = "INVALID_BRANCH"
	ErrorTypeInvalidEdge      ValidationErrorType = "INVALID_EDGE"
	ErrorTypeDuplicateNode    ValidationErrorType = "DUPLICATE_NODE"
)

// ValidationError represents a graph validation error.
type ValidationError struct {
	Type    ValidationErrorType
	Node    string
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if e.Node != "" {
		return fmt.Sprintf("[%s] node=%q: %s", e.Type, e.Node, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// ValidationLevel determines the strictness of validation.
type ValidationLevel int

const (
	// ValidationLevelNone skips all validation.
	ValidationLevelNone ValidationLevel = iota
	// ValidationLevelBasic performs basic structural validation.
	ValidationLevelBasic
	// ValidationLevelStrict performs comprehensive validation.
	ValidationLevelStrict
)

// ValidationOptions configures graph validation behavior.
type ValidationOptions struct {
	Level                  ValidationLevel
	SkipValidation         bool
	AllowCycles            bool
	AllowDisconnectedNodes bool
}

// DefaultValidationOptions returns the default validation configuration.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Level:                  ValidationLevelBasic,
		SkipValidation:         false,
		AllowCycles:            true, // BSP model handles cycles
		AllowDisconnectedNodes: false,
	}
}

// StrictValidationOptions returns strict validation configuration.
func StrictValidationOptions() ValidationOptions {
	return ValidationOptions{
		Level:                  ValidationLevelStrict,
		SkipValidation:         false,
		AllowCycles:            false,
		AllowDisconnectedNodes: false,
	}
}

// validator validates graph structure.
type validator struct {
	opts ValidationOptions
}

// newValidator creates a new validator with the given options.
func newValidator(opts ValidationOptions) *validator {
	return &validator{opts: opts}
}

// validate performs validation checks on the graph.
func (v *validator) validate(g *Graph) []ValidationError {
	var errors []ValidationError

	// Basic structural validation
	errors = append(errors, v.validateNodes(g)...)
	errors = append(errors, v.validateEdges(g)...)
	errors = append(errors, v.validateCommandTargets(g)...) // Validate DeclaredTargets instead of branches
	errors = append(errors, v.validateEntryPoints(g)...)

	// Strict validation
	if v.opts.Level >= ValidationLevelStrict {
		if !v.opts.AllowCycles {
			errors = append(errors, v.validateAcyclic(g)...)
		}
		if !v.opts.AllowDisconnectedNodes {
			errors = append(errors, v.validateConnectivity(g)...)
		}
	}

	return errors
}

// validateNodes checks that all nodes are valid.
func (v *validator) validateNodes(g *Graph) []ValidationError {
	var errors []ValidationError

	if len(g.Nodes) == 0 {
		errors = append(errors, ValidationError{
			Type:    ErrorTypeMissingNode,
			Message: "graph has no nodes",
		})
	}

	// Check for duplicate node names
	seen := make(map[string]bool)
	for name := range g.Nodes {
		if seen[name] {
			errors = append(errors, ValidationError{
				Type:    ErrorTypeDuplicateNode,
				Node:    name,
				Message: "duplicate node name",
			})
		}
		seen[name] = true
	}

	return errors
}

// validateEdges checks that the entry point references an existing node.
func (v *validator) validateEdges(g *Graph) []ValidationError {
	var errors []ValidationError

	// Check all entry points exist
	for _, entryPoint := range g.EntryPoints {
		if entryPoint != EndNode {
			if _, exists := g.Nodes[entryPoint]; !exists {
				errors = append(errors, ValidationError{
					Type:    ErrorTypeInvalidEdge,
					Node:    StartNode,
					Message: fmt.Sprintf("entry point references non-existent node %q", entryPoint),
				})
			}
		}
	}

	return errors
}

// validateCommandTargets checks that all node DeclaredTargets are valid.
func (v *validator) validateCommandTargets(g *Graph) []ValidationError {
	var errors []ValidationError

	// Validate each node's DeclaredTargets
	for name, node := range g.Nodes {
		targets := node.Targets()
		for _, target := range targets {
			// Allow virtual START/END nodes
			if target != StartNode && target != EndNode {
				if _, exists := g.Nodes[target]; !exists {
					errors = append(errors, ValidationError{
						Type:    ErrorTypeInvalidBranch, // Reuse error type
						Node:    name,
						Message: fmt.Sprintf("node declares non-existent target %q", target),
					})
				}
			}
		}
	}

	return errors
}

// validateEntryPoints checks that START and END nodes are properly connected.
func (v *validator) validateEntryPoints(g *Graph) []ValidationError {
	var errors []ValidationError

	// Check for at least one entry point
	if len(g.EntryPoints) == 0 {
		errors = append(errors, ValidationError{
			Type:    ErrorTypeInvalidEntryNode,
			Message: fmt.Sprintf("graph has no entry points (no edges from %s node)", StartNode),
		})
	}

	// Check for END node connections (via Command node targets)
	hasEndConnection := false
	for _, node := range g.Nodes {
		targets := node.Targets()
		for _, target := range targets {
			if target == EndNode {
				hasEndConnection = true
				break
			}
		}
		if hasEndConnection {
			break
		}
	}

	if !hasEndConnection {
		errors = append(errors, ValidationError{
			Type:    ErrorTypeInvalidEndNode,
			Message: fmt.Sprintf("graph has no routing to %s node", EndNode),
		})
	}

	return errors
}

// validateAcyclic checks that the graph is acyclic using DFS.
func (v *validator) validateAcyclic(g *Graph) []ValidationError {
	var errors []ValidationError
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(string) bool
	hasCycle = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		//nolint:nestif // Acceptable nesting for cycle detection algorithm
		// Check entry point edges
		if node == StartNode {
			for _, entryPoint := range g.EntryPoints {
				if !visited[entryPoint] {
					if hasCycle(entryPoint) {
						return true
					}
				} else if recStack[entryPoint] {
					errors = append(errors, ValidationError{
						Type:    ErrorTypeCycle,
						Node:    node,
						Message: fmt.Sprintf("cycle detected: %s -> %s", node, entryPoint),
					})
					return true
				}
			}
		}
		// Check node targets
		//nolint:nestif // Cycle detection requires nested traversal
		if n, exists := g.Nodes[node]; exists {
			for _, target := range n.Targets() {
				if !visited[target] {
					if hasCycle(target) {
						return true
					}
				} else if recStack[target] {
					errors = append(errors, ValidationError{
						Type:    ErrorTypeCycle,
						Node:    node,
						Message: fmt.Sprintf("cycle detected: %s -> %s", node, target),
					})
					return true
				}
			}
		}

		recStack[node] = false
		return false
	}

	for node := range g.Nodes {
		if !visited[node] {
			hasCycle(node)
		}
	}

	return errors
}

// validateConnectivity checks that all nodes are reachable from START.
func (v *validator) validateConnectivity(g *Graph) []ValidationError {
	var errors []ValidationError

	// Build adjacency list
	adj := make(map[string][]string)
	// Add entry point edges
	adj[StartNode] = append(adj[StartNode], g.EntryPoints...)
	// Add node targets
	for name, node := range g.Nodes {
		targets := node.Targets()
		if len(targets) > 0 {
			adj[name] = append(adj[name], targets...)
		}
	}

	// BFS from START node
	visited := make(map[string]bool)
	queue := []string{StartNode}
	visited[StartNode] = true

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for _, next := range adj[node] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}

	// Check for disconnected nodes (excluding virtual START/END)
	for node := range g.Nodes {
		if !visited[node] && node != StartNode && node != EndNode {
			errors = append(errors, ValidationError{
				Type:    ErrorTypeDisconnected,
				Node:    node,
				Message: "node is not reachable from START",
			})
		}
	}

	return errors
}
