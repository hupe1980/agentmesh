package graph

import (
	"fmt"
)

// ValidationErrorType classifies validation errors.
type ValidationErrorType string

// Validation error type constants.
const (
	ErrorTypeCycle            ValidationErrorType = "CYCLE"
	ErrorTypeDisconnected     ValidationErrorType = "DISCONNECTED"
	ErrorTypeDuplicateKey     ValidationErrorType = "DUPLICATE_KEY"
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
	// ValidationLevelStrict performs comprehensive validation including cycle detection.
	ValidationLevelStrict
)

// ValidationOptions configures graph validation behavior.
type ValidationOptions struct {
	Level                  ValidationLevel
	AllowCycles            bool
	AllowDisconnectedNodes bool
}

// DefaultValidationOptions returns the default validation configuration.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Level:                  ValidationLevelBasic,
		AllowCycles:            true, // BSP model handles cycles
		AllowDisconnectedNodes: false,
	}
}

// StrictValidationOptions returns strict validation configuration.
func StrictValidationOptions() ValidationOptions {
	return ValidationOptions{
		Level:                  ValidationLevelStrict,
		AllowCycles:            false,
		AllowDisconnectedNodes: false,
	}
}

// Validate performs validation on the graph and returns any errors found.
// This is useful for more detailed error reporting than Build() provides.
func (g *Graph[I, O]) Validate(opts ...ValidationOptions) []ValidationError {
	opt := DefaultValidationOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt.Level == ValidationLevelNone {
		return nil
	}

	var errors []ValidationError

	// Key validation
	errors = append(errors, g.validateKeys()...)

	// Basic validation
	errors = append(errors, g.validateNodes()...)
	errors = append(errors, g.validateEdges()...)
	errors = append(errors, g.validateEntryPoints()...)

	// Strict validation
	if opt.Level >= ValidationLevelStrict {
		if !opt.AllowCycles {
			errors = append(errors, g.detectCycles()...)
		}
		if !opt.AllowDisconnectedNodes {
			errors = append(errors, g.detectDisconnected()...)
		}
	}

	return errors
}

// validateKeys checks for duplicate key names.
func (g *Graph[I, O]) validateKeys() []ValidationError {
	var errors []ValidationError

	seen := make(map[string]bool)
	for _, key := range g.keys {
		name := key.Name()
		if name == "" {
			continue
		}

		if seen[name] {
			errors = append(errors, ValidationError{
				Type:    ErrorTypeDuplicateKey,
				Message: fmt.Sprintf("duplicate key: %s", name),
			})
		}

		seen[name] = true
	}

	return errors
}

// validateNodes checks that all nodes have been defined.
func (g *Graph[I, O]) validateNodes() []ValidationError {
	var errors []ValidationError

	if len(g.nodes) == 0 {
		errors = append(errors, ValidationError{
			Type:    ErrorTypeMissingNode,
			Message: "graph has no nodes",
		})
	}

	return errors
}

// validateEdges checks that all edge targets are valid.
func (g *Graph[I, O]) validateEdges() []ValidationError {
	var errors []ValidationError

	for _, n := range g.nodes {
		for _, target := range n.targets {
			if target == END {
				continue
			}
			if _, ok := g.nodes[target]; !ok {
				errors = append(errors, ValidationError{
					Type:    ErrorTypeInvalidEdge,
					Node:    n.name,
					Message: fmt.Sprintf("target node %q does not exist", target),
				})
			}
		}
	}

	return errors
}

// validateEntryPoints checks that entry points are valid.
func (g *Graph[I, O]) validateEntryPoints() []ValidationError {
	var errors []ValidationError

	if len(g.entryPoints) == 0 {
		errors = append(errors, ValidationError{
			Type:    ErrorTypeInvalidEntryNode,
			Message: "no entry point defined",
		})
		return errors
	}

	for _, ep := range g.entryPoints {
		if _, ok := g.nodes[ep]; !ok {
			errors = append(errors, ValidationError{
				Type:    ErrorTypeInvalidEntryNode,
				Node:    ep,
				Message: fmt.Sprintf("entry point %q does not exist", ep),
			})
		}
	}

	return errors
}

// detectCycles detects cycles in the graph using DFS.
func (g *Graph[I, O]) detectCycles() []ValidationError {
	var errors []ValidationError

	// Track visited and recursion stack
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(name string) bool
	dfs = func(name string) bool {
		visited[name] = true
		recStack[name] = true

		n, ok := g.nodes[name]
		if !ok {
			return false
		}

		for _, target := range n.targets {
			if target == END {
				continue
			}
			if !visited[target] {
				if dfs(target) {
					return true
				}
			} else if recStack[target] {
				// Found a cycle
				errors = append(errors, ValidationError{
					Type:    ErrorTypeCycle,
					Node:    name,
					Message: fmt.Sprintf("cycle detected: %s -> %s", name, target),
				})
				return true
			}
		}

		recStack[name] = false
		return false
	}

	// Start DFS from each entry point
	for _, ep := range g.entryPoints {
		if !visited[ep] {
			dfs(ep)
		}
	}

	return errors
}

// detectDisconnected finds nodes that are not reachable from entry points.
func (g *Graph[I, O]) detectDisconnected() []ValidationError {
	var errors []ValidationError

	// BFS from entry points to find all reachable nodes
	reachable := make(map[string]bool)
	queue := make([]string, 0, len(g.entryPoints))
	queue = append(queue, g.entryPoints...)

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if reachable[name] {
			continue
		}
		reachable[name] = true

		n, ok := g.nodes[name]
		if !ok {
			continue
		}

		for _, target := range n.targets {
			if target == END {
				continue
			}
			if !reachable[target] {
				queue = append(queue, target)
			}
		}
	}

	// Check for unreachable nodes
	for name := range g.nodes {
		if !reachable[name] {
			errors = append(errors, ValidationError{
				Type:    ErrorTypeDisconnected,
				Node:    name,
				Message: fmt.Sprintf("node %q is not reachable from any entry point", name),
			})
		}
	}

	return errors
}
