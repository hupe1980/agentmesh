package compile

import (
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Reserved node names
const (
	// StartNode is the graph entry point
	StartNode = "__start__"

	// EndNode is the graph exit point
	EndNode = "__end__"
)

// CompiledGraph represents a validated and compiled graph ready for execution.
// It contains the original graph structure plus computed topology information.
type CompiledGraph struct {
	// Original graph structure
	Graph *graph.Graph

	// Computed topology (internal, optimized for execution)
	Topology *executionTopology

	// Manager for execution (unified state management)
	Manager state.Manager

	// Quick lookups
	StartNode string
	EndNode   string
}

// Compile validates and compiles a graph into an executable form.
// It performs topological validation and prepares the graph for execution.
func Compile(g *graph.Graph, manager state.Manager, opts ...CompileOption) (*CompiledGraph, error) {
	cfg := &compileConfig{
		validationOpts: DefaultValidationOptions(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Validate graph structure
	validator := NewValidator(cfg.validationOpts)
	errors := validator.Validate(g)
	if len(errors) > 0 {
		return nil, formatValidationErrors(errors)
	}

	// Compute topology (graph.Graph uses Branches, not Conditionals)
	topo := computeTopology(g.Nodes, g.Edges, g.Branches)

	return &CompiledGraph{
		Graph:     g,
		Topology:  topo,
		Manager:   manager,
		StartNode: StartNode,
		EndNode:   EndNode,
	}, nil
}

// compileConfig holds compilation configuration.
type compileConfig struct {
	validationOpts ValidationOptions
}

// CompileOption configures compilation behavior.
type CompileOption func(*compileConfig)

// WithValidation sets custom validation options.
func WithValidation(opts ValidationOptions) CompileOption {
	return func(c *compileConfig) {
		c.validationOpts = opts
	}
}

// WithStrictValidation enables strict validation mode.
func WithStrictValidation() CompileOption {
	return func(c *compileConfig) {
		c.validationOpts = StrictValidationOptions()
	}
}

// WithoutValidation disables validation (use with caution).
// Only use this for trusted graphs or when validation overhead is unacceptable.
func WithoutValidation() CompileOption {
	return func(c *compileConfig) {
		c.validationOpts = ValidationOptions{
			SkipValidation: true,
		}
	}
}

// formatValidationErrors formats validation errors into a single error message.
func formatValidationErrors(errors []ValidationError) error {
	if len(errors) == 0 {
		return nil
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("graph validation failed with %d error(s):\n", len(errors)))
	for i, err := range errors {
		msg.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return fmt.Errorf("%s", msg.String())
}

// Nodes returns all nodes in the graph.
func (cg *CompiledGraph) Nodes() map[string]*graph.Node {
	return cg.Graph.Nodes
}

// GetNode returns a node by name, or nil if not found.
func (cg *CompiledGraph) GetNode(name string) *graph.Node {
	return cg.Graph.Nodes[name]
}
