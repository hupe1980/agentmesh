package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Builder provides a fluent API for constructing graphs.
// It wraps Graph and returns itself from most methods to enable method chaining.
//
// Example:
//
//	compiled, err := graph.NewBuilder().
//	    WithState(state).
//	    AddNode(&graph.Node{Name: "start", RunFunc: startFunc}).
//	    AddNode(&graph.Node{Name: "process", RunFunc: processFunc}).
//	    AddEdge(graph.StartNode, "start").
//	    AddEdge("start", "process").
//	    AddEdge("process", graph.EndNode).
//	    Compile()
type Builder struct {
	graph *Graph
	err   error // Accumulated error
}

// NewBuilder creates a new graph builder with default empty state.
// Returns an error if the graph cannot be initialized (currently always succeeds).
func NewBuilder() (*Builder, error) {
	graph, err := NewGraph(nil)
	if err != nil {
		return nil, err
	}
	return &Builder{
		graph: graph,
	}, nil
}

// SetStateManager configures the graph state manager.
// This is the primary API for setting state - fully supports any StateManager implementation.
func (b *Builder) SetStateManager(stateManager StateManager) *Builder {
	if b.err != nil {
		return b
	}
	b.graph.stateManager = stateManager
	return b
}

// WithState is a convenience method that wraps SetStateManager.
// It accepts any StateManager implementation.
func (b *Builder) WithState(stateManager StateManager) *Builder {
	return b.SetStateManager(stateManager)
}

// WithMaxMessages configures the maximum number of messages to retain.
// Default is 0 (unlimited). This applies to the standard "messages" channel.
// Note: This only works with the default *ChannelState implementation.
func (b *Builder) WithMaxMessages(maxMessages int) *Builder {
	if b.err != nil {
		return b
	}

	// This is a convenience method that only works with *ChannelState
	if state, ok := b.graph.stateManager.(*ChannelState); ok {
		state.SetMaxMessages(maxMessages)
	}
	// Note: If using a custom StateManager, the caller must configure it directly
	// This is intentional - custom implementations handle their own configuration

	return b
}

// WithInitialChannels initializes the state with custom channels.
// Note: This only works with the default *ChannelState implementation.
// For custom StateManager implementations, configure them before passing to SetStateManager.
//
// Example:
//
//	state := graph.NewStateManager(0).(*graph.ChannelState)
//	state.AddChannel(channel.NewLastValueChannel("status"))
//	builder.WithState(state)
func (b *Builder) WithInitialChannels(configFn func(*ChannelState)) *Builder {
	if b.err != nil {
		return b
	}

	// Ensure we have a state manager
	if b.graph.stateManager == nil {
		sm, err := NewStateManager(0)
		if err != nil {
			b.err = fmt.Errorf("failed to create state manager: %w", err)
			return b
		}
		b.graph.stateManager = sm
	}

	// This convenience method only works with *ChannelState
	if state, ok := b.graph.stateManager.(*ChannelState); ok && configFn != nil {
		configFn(state)
	}
	// For custom StateManagers, silently skip - they should be pre-configured

	return b
}

// WithExecutor sets a custom execution strategy for the graph.
// By default, graphs use Pregel BSP parallel execution. Use this to:
//   - Switch to SimpleGraphExecutor for debugging (sequential, single-threaded)
//   - Implement custom execution strategies
//   - Use alternative distributed execution backends
//
// Example:
//
//	builder.WithExecutor(graph.NewSimpleGraphExecutor()) // Sequential debugging mode
func (b *Builder) WithExecutor(executor Executor) *Builder {
	if b.err != nil {
		return b
	}
	b.graph.executor = executor
	return b
}

// AddNode adds a node to the graph.
// Returns the builder for chaining.
func (b *Builder) AddNode(node *Node) *Builder {
	if b.err != nil {
		return b
	}
	if err := b.graph.AddNode(node); err != nil {
		b.err = err
	}
	return b
}

// Node creates and adds a node with the given name and run function.
// Convenience method to avoid creating Node structs.
//
// Example:
//
//	builder.Node("process", func(ctx context.Context, s state.Writer) (*NodeResult, error) {
//	    // process logic
//	    return &NodeResult{Updates: map[string]any{"done": true}}, nil
//	})
func (b *Builder) Node(name string, runFunc func(context.Context, state.Writer) (*NodeResult, error)) *Builder {
	return b.AddNode(&Node{Name: name, RunFunc: runFunc})
}

// AddEdge adds a directed edge from one node to another.
func (b *Builder) AddEdge(from, to string) *Builder {
	if b.err != nil {
		return b
	}
	b.graph.AddEdge(from, to)
	return b
}

// AddEdges adds multiple edges from one source node to multiple targets.
//
// Example:
//
//	builder.AddEdges("router", []string{"path_a", "path_b", "path_c"})
func (b *Builder) AddEdges(from string, targets []string) *Builder {
	if b.err != nil {
		return b
	}
	for _, to := range targets {
		b.graph.AddEdge(from, to)
	}
	return b
}

// AddConditionalEdges adds conditional branching from a node.
func (b *Builder) AddConditionalEdges(from string, condition func(context.Context, state.Reader) []string, targets []string) *Builder {
	if b.err != nil {
		return b
	}
	b.graph.AddConditionalEdges(from, condition, targets)
	return b
}

// ConditionalRoute adds a simpler conditional edge that returns a single target.
// Automatically wraps the condition to return []string for the graph.
//
// Example:
//
//	builder.ConditionalRoute("router", func(ctx context.Context, s state.Reader) (string, error) {
//	    if s.Get("valid").(bool) {
//	        return "success", nil
//	    }
//	    return "failure", nil
//	}, []string{"success", "failure"})
func (b *Builder) ConditionalRoute(from string, condition func(context.Context, state.Reader) (string, error), targets []string) *Builder {
	if b.err != nil {
		return b
	}
	wrappedCondition := func(ctx context.Context, s state.Reader) []string {
		target, err := condition(ctx, s)
		if err != nil {
			// Default to first target or empty
			if len(targets) > 0 {
				return []string{targets[0]}
			}
			return []string{}
		}
		return []string{target}
	}
	b.graph.AddConditionalEdges(from, wrappedCondition, targets)
	return b
}

// StartTo creates an edge from the start node to the given target.
// Convenience method for common pattern.
func (b *Builder) StartTo(target string) *Builder {
	return b.AddEdge(StartNode, target)
}

// ToEnd creates an edge from the given source to the end node.
// Convenience method for common pattern.
func (b *Builder) ToEnd(from string) *Builder {
	return b.AddEdge(from, EndNode)
}

// Chain creates a linear sequence of nodes.
// Automatically creates edges from start to each node in order to end.
//
// Example:
//
//	builder.Chain("fetch", "validate", "process", "store")
//	// Creates: START -> fetch -> validate -> process -> store -> END
func (b *Builder) Chain(nodeNames ...string) *Builder {
	if b.err != nil {
		return b
	}
	if len(nodeNames) == 0 {
		return b
	}

	// Start -> first node
	b.AddEdge(StartNode, nodeNames[0])

	// Chain nodes
	for i := 0; i < len(nodeNames)-1; i++ {
		b.AddEdge(nodeNames[i], nodeNames[i+1])
	}

	// Last node -> End
	b.AddEdge(nodeNames[len(nodeNames)-1], EndNode)

	return b
}

// Parallel creates edges from a source to multiple targets in parallel,
// then converges all targets to a single destination.
//
// Example:
//
//	builder.Parallel("router", []string{"task_a", "task_b", "task_c"}, "aggregator")
//	// Creates: router -> task_a -> aggregator
//	//          router -> task_b -> aggregator
//	//          router -> task_c -> aggregator
func (b *Builder) Parallel(source string, tasks []string, destination string) *Builder {
	if b.err != nil {
		return b
	}
	for _, task := range tasks {
		b.AddEdge(source, task)
		b.AddEdge(task, destination)
	}
	return b
}

// AddSubgraph embeds a compiled subgraph as a node.
// Convenience method wrapping Compiled.AsNode().
func (b *Builder) AddSubgraph(name string, subgraph *Compiled) *Builder {
	return b.AddNode(subgraph.AsNode(name))
}

// Graph returns the underlying graph being built.
// Use this if you need direct access to graph methods not wrapped by the builder.
func (b *Builder) Graph() *Graph {
	return b.graph
}

// Compile validates and compiles the graph, returning a Compiled.
// Returns any accumulated error from previous builder operations.
func (b *Builder) Compile() (*Compiled, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.graph.Compile()
}

// MustCompile is like Compile but panics on error.
// Useful for static graph construction where errors indicate programmer mistakes.
func (b *Builder) MustCompile() *Compiled {
	compiled, err := b.Compile()
	if err != nil {
		panic(err)
	}
	return compiled
}
