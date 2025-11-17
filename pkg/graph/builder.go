package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// ErrNoCompileFunc is returned when trying to compile a builder without a compile function.
var ErrNoCompileFunc = fmt.Errorf("no compile function registered; use WithCompileFunc option or SetCompileFunc method")

// Builder provides a fluent API for constructing graphs.
type Builder struct {
	graph       *Graph
	compileFunc func(*Graph) (MessageRunnable, error)
}

// BuilderOption is a functional option for configuring the Builder.
type BuilderOption func(*Builder) error

// NewBuilder creates a new graph builder with the given options.
func NewBuilder(opts ...BuilderOption) (*Builder, error) {
	// Create a default state manager
	manager := state.NewManager()

	graph, err := NewGraph(manager)
	if err != nil {
		return nil, err
	}

	b := &Builder{
		graph: graph,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}

	return b, nil
}

// WithManager sets a custom state manager for the builder.
func WithManager(manager *state.Manager) BuilderOption {
	return func(b *Builder) error {
		graph, err := NewGraph(manager)
		if err != nil {
			return err
		}
		b.graph = graph
		return nil
	}
}

// WithState is deprecated. Use WithManager instead.
// Kept for temporary compatibility during migration.
func WithState(st *state.State) BuilderOption {
	return func(b *Builder) error {
		// Create a manager (state is no longer used directly)
		manager := state.NewManager()
		graph, err := NewGraph(manager)
		if err != nil {
			return err
		}
		b.graph = graph
		return nil
	}
}

// WithCompileFunc sets a custom compile function for the builder.
// This is used to avoid import cycles with the exec package.
//
// Example:
//
//	builder := graph.NewBuilder(graph.WithCompileFunc(exec.CompileGraph))
func WithCompileFunc(compileFunc func(*Graph) (MessageRunnable, error)) BuilderOption {
	return func(b *Builder) error {
		b.compileFunc = compileFunc
		return nil
	}
}

// Node adds a node to the graph with the given name and run function.
// Any errors will be caught during graph compilation in Build().
func (b *Builder) Node(name string, runFunc func(ctx context.Context, view *state.ReadView) (*NodeResult, error)) *Builder {
	// Errors are validated during graph compilation
	_ = b.graph.AddNode(&Node{
		Name:    name,
		RunFunc: runFunc,
	})
	return b
}

// NodeWithRetry adds a node to the graph with a retry policy.
// This is a convenience method for adding a node with automatic retry behavior.
// Any errors will be caught during graph compilation in Build().
//
// Example:
//
//	builder.NodeWithRetry("api_call", apiFunc,
//	    graph.NewRetryPolicy().
//	        WithMaxAttempts(5).
//	        WithExponentialBackoff(time.Second, 2.0).
//	        Build())
func (b *Builder) NodeWithRetry(name string, runFunc func(ctx context.Context, view *state.ReadView) (*NodeResult, error), retryPolicy *RetryPolicy) *Builder {
	// Errors are validated during graph compilation
	_ = b.graph.AddNode(&Node{
		Name:        name,
		RunFunc:     runFunc,
		RetryPolicy: retryPolicy,
	})
	return b
}

// SetNodeRetryPolicy sets or updates the retry policy for an existing node.
// Returns an error if the node doesn't exist.
//
// Example:
//
//	builder.Node("process", processFunc)
//	builder.SetNodeRetryPolicy("process",
//	    graph.NewRetryPolicy().WithMaxAttempts(3).Build())
func (b *Builder) SetNodeRetryPolicy(name string, retryPolicy *RetryPolicy) error {
	node, exists := b.graph.Nodes[name]
	if !exists {
		return fmt.Errorf("node not found: %s", name)
	}
	node.RetryPolicy = retryPolicy
	return nil
}

// AddEdge adds a directed edge between two nodes.
func (b *Builder) AddEdge(from, to string) *Builder {
	b.graph.AddEdge(from, to)
	return b
}

// AddConditionalEdges adds conditional routing based on runtime state.
func (b *Builder) AddConditionalEdges(from string, condition func(context.Context, *state.ReadView) []string, targets []string) *Builder {
	b.graph.AddConditionalEdges(from, condition, targets)
	return b
}

// Graph returns the underlying graph.
func (b *Builder) Graph() *Graph {
	return b.graph
}

// Manager returns the graph's state manager.
func (b *Builder) Manager() *state.Manager {
	return b.graph.Manager()
}

// State is deprecated. Use Manager() instead.
// Returns nil as State is no longer used directly.
func (b *Builder) State() *state.State {
	return nil
}

// Compile compiles the graph into a MessageRunnable using the registered compile function.
// If no compile function is registered, returns an error.
// To set a compile function, use WithCompileFunc option or call SetCompileFunc.
//
// Example:
//
//	import "github.com/hupe1980/agentmesh/pkg/exec"
//	builder.SetCompileFunc(exec.CompileGraph)
//	compiled, err := builder.Compile()
func (b *Builder) Compile() (MessageRunnable, error) {
	if b.compileFunc == nil {
		return nil, ErrNoCompileFunc
	}
	return b.compileFunc(b.graph)
}

// CompileMessageRunnable compiles the graph into a MessageRunnable.
// This is an alias for Compile() to maintain API compatibility.
func (b *Builder) CompileMessageRunnable() (MessageRunnable, error) {
	return b.Compile()
}

// SetCompileFunc sets the compile function after builder creation.
func (b *Builder) SetCompileFunc(compileFunc func(*Graph) (MessageRunnable, error)) {
	b.compileFunc = compileFunc
}

// Build returns the underlying graph without compiling.
func (b *Builder) Build() *Graph {
	return b.graph
}
