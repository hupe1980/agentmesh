package graph

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Compiled represents a validated, executable graph.
// Generic over Input and Output types for type-safe execution.
//
// Type parameters:
//   - I: Input type accepted by the executor
//   - O: Output type produced by the executor
type Compiled[I, O any] struct {
	graph    *Graph         // Original graph structure
	topology *topology      // Internal: execution order, dependencies
	executor Executor[I, O] // Pluggable execution strategy
	manager  *state.Manager // State management
}

// Run executes the compiled graph with the given input.
// Returns an iterator that yields outputs and errors as execution progresses.
//
// Example:
//
//	for output, err := range compiled.Run(ctx, input) {
//	    if err != nil {
//	        return fmt.Errorf("execution failed: %w", err)
//	    }
//	    fmt.Printf("Output: %+v\n", output)
//	}
func (c *Compiled[I, O]) Run(ctx context.Context, input I, opts ...RunOption) iter.Seq2[O, error] {
	return c.executor.Run(ctx, c, input, opts...)
}

// Graph returns the original graph structure.
func (c *Compiled[I, O]) Graph() *Graph {
	return c.graph
}

// Manager returns the state manager.
func (c *Compiled[I, O]) Manager() *state.Manager {
	return c.manager
}

// GetNodes returns all node names in the graph.
func (c *Compiled[I, O]) GetNodes() []string {
	names := make([]string, 0, len(c.graph.Nodes))
	for name := range c.graph.Nodes {
		names = append(names, name)
	}
	return names
}

// GetIncomingEdges returns the names of nodes that have edges to the given node.
func (c *Compiled[I, O]) GetIncomingEdges(node string) []string {
	incoming := make([]string, 0)
	// Check entry points
	for _, ep := range c.graph.EntryPoints {
		if ep == node {
			incoming = append(incoming, StartNode)
			break
		}
	}
	// Check all node targets
	for name, n := range c.graph.Nodes {
		for _, target := range n.Targets() {
			if target == node {
				incoming = append(incoming, name)
			}
		}
	}
	return incoming
}

// GetOutgoingEdges returns the names of nodes that the given node has edges to.
// Returns the internal slice directly as topology is immutable after compilation.
// Returns nil if the node has no outgoing edges.
func (c *Compiled[I, O]) GetOutgoingEdges(node string) []string {
	return c.topology.outgoing[node]
}

// GetNodeInfo returns detailed information about a specific node.
// Delegates to the underlying graph.
func (c *Compiled[I, O]) GetNodeInfo(name string) (*NodeInfo, error) {
	return c.graph.GetNodeInfo(name)
}

// GetTopology returns a comprehensive view of the graph structure.
// Delegates to the underlying graph.
func (c *Compiled[I, O]) GetTopology() *Topology {
	return c.graph.GetTopology()
}

// GetMetrics returns static graph metrics.
// Delegates to the underlying graph.
func (c *Compiled[I, O]) GetMetrics() *Metrics {
	return c.graph.GetMetrics()
}

// GetNodeDependencies returns dependency information for a specific node.
// Delegates to the underlying graph.
func (c *Compiled[I, O]) GetNodeDependencies(name string) (*NodeDependencies, error) {
	return c.graph.GetNodeDependencies(name)
}

// MermaidFlowchart generates a Mermaid flowchart representation.
func (c *Compiled[I, O]) MermaidFlowchart(direction string) string {
	if direction == "" {
		direction = "TD"
	}

	var result string
	result += fmt.Sprintf("graph %s\n", direction)

	// Add nodes
	for nodeName := range c.graph.Nodes {
		// Style based on node type
		switch nodeName {
		case StartNode:
			result += fmt.Sprintf("    %s([START])\n", nodeName)
		case EndNode:
			result += fmt.Sprintf("    %s([END])\n", nodeName)
		default:
			// Check if node has multiple targets (branching)
			node := c.graph.Nodes[nodeName]
			targets := node.Targets()
			if len(targets) > 1 {
				result += fmt.Sprintf("    %s{%s}\n", nodeName, nodeName)
			} else {
				result += fmt.Sprintf("    %s[%s]\n", nodeName, nodeName)
			}
		}
	}

	// Add entry point edges (from SetEntryPoint)
	for _, entryPoint := range c.graph.EntryPoints {
		result += fmt.Sprintf("    %s --> %s\n", StartNode, entryPoint)
	}

	// Add Command routing edges (dashed lines for DeclaredTargets)
	// Skip START/END nodes as they don't have user-defined targets
	for name, node := range c.graph.Nodes {
		if name == StartNode || name == EndNode {
			continue
		}

		// Get declared targets from Command nodes
		targets := node.Targets()
		for _, target := range targets {
			// Use dashed line to indicate Command routing (dynamic decision)
			result += fmt.Sprintf("    %s -.-> %s\n", name, target)
		}
	}

	return result
}

// ApplyState applies state updates to the graph's state manager.
func (c *Compiled[I, O]) ApplyState(ctx context.Context, updates state.Updates) error {
	if err := c.manager.ApplyUpdates(ctx, updates); err != nil {
		return fmt.Errorf("%w: %w", ErrStateApply, err)
	}
	return nil
}

// CurrentSuperstep returns the current superstep number from the executor.
// Returns 0 if the executor doesn't track supersteps or hasn't started.
// Useful for resuming execution from where it was paused.
func (c *Compiled[I, O]) CurrentSuperstep() int64 {
	// Try to get metrics from PregelExecutor
	if pregel, ok := c.executor.(*PregelExecutor[I, O]); ok && pregel.metrics != nil {
		snapshot := pregel.metrics.Snapshot()
		return snapshot.CurrentSuperstep
	}
	return 0
}

// Compile validates and prepares a graph for execution.
// Type parameters I, O are inferred from the executor.
//
// Example:
//
//	executor := NewPregelExecutor[[]message.Message, message.Message](...)
//	compiled, err := graph.Compile(g, executor)
//	if err != nil {
//	    return fmt.Errorf("compilation failed: %w", err)
//	}
func Compile[I, O any](g *Graph, executor Executor[I, O], opts ...CompileOption) (*Compiled[I, O], error) {
	if executor == nil {
		return nil, ErrExecutorNil
	}

	// Setup configuration
	cfg := &compileConfig{
		validationOpts: DefaultValidationOptions(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Validate graph structure
	if !cfg.validationOpts.SkipValidation {
		validator := newValidator(cfg.validationOpts)
		errors := validator.validate(g)
		if len(errors) > 0 {
			return nil, formatValidationErrors(errors)
		}
	}

	// Compute topology
	topo := computeTopology(g.Nodes, g.EntryPoints)

	// Freeze the state manager to prevent further schema modifications
	// This enforces the write-once, read-many pattern for optimal performance
	g.manager.Freeze()

	return &Compiled[I, O]{
		graph:    g,
		topology: topo,
		executor: executor,
		manager:  g.manager,
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

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("graph validation failed with %d error(s):\n", len(errors)))

	for i, err := range errors {
		builder.WriteString(fmt.Sprintf("  %d. [%s] ", i+1, err.Type))
		if err.Node != "" {
			builder.WriteString(fmt.Sprintf("node=%q ", err.Node))
		}
		builder.WriteString(err.Message)
		builder.WriteString("\n")
	}

	return fmt.Errorf("%w: %s", ErrValidation, builder.String())
}
