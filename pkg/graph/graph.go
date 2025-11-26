package graph

import (
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Graph represents a mutable computational graph with nodes and edges.
type Graph struct {
	Nodes           map[string]Node
	NodeConfigs     map[string]*NodeConfig // Execution policies per node
	InterruptBefore []string               // Nodes to interrupt before execution
	InterruptAfter  []string               // Nodes to interrupt after execution
	EntryPoints     []string               // Names of entry point nodes (parallel start)
	manager         *state.Manager
}

// NewGraph creates a new graph with the given state manager.
func NewGraph(manager *state.Manager) (*Graph, error) {
	if err := validate.NotNil(manager, "manager"); err != nil {
		return nil, err
	}
	return &Graph{
		Nodes:           make(map[string]Node),
		NodeConfigs:     make(map[string]*NodeConfig),
		InterruptBefore: make([]string, 0), // Initialize interrupt lists
		InterruptAfter:  make([]string, 0),
		EntryPoints:     make([]string, 0), // Initialize entry points
		manager:         manager,
	}, nil
}

// AddNode adds a node to the graph with optional execution policies.
//
// Example with retry policy:
//
//	g.AddNode(myNode, WithRetryPolicy(&RetryPolicy{
//	    MaxAttempts: 3,
//	    Backoff: ExponentialBackoff(100 * time.Millisecond),
//	}))
func (g *Graph) AddNode(n Node, opts ...NodeOption) error {
	if err := validate.All(
		validate.NotNil(n, "node"),
		validate.NotEmpty(n.Name(), "node name"),
	); err != nil {
		return err
	}

	// Create config with defaults
	config := defaultNodeConfig()

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Validate config
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid node config for %q: %w", n.Name(), err)
	}

	g.Nodes[n.Name()] = n
	g.NodeConfigs[n.Name()] = config

	return nil
}

// SetEntryPoint adds entry point nodes to the graph.
// Entry points are the nodes that will be executed first when the graph runs,
// all in parallel in the same superstep.
// The entry points are validated at compile time, so nodes can be added after
// calling SetEntryPoint (useful for builder pattern).
func (g *Graph) SetEntryPoint(target string) error {
	if err := validate.NotEmpty(target, "entry point target"); err != nil {
		return err
	}

	// Check for duplicates
	for _, ep := range g.EntryPoints {
		if ep == target {
			return fmt.Errorf("entry point %q already exists", target)
		}
	}

	g.EntryPoints = append(g.EntryPoints, target)
	return nil
}

// Manager returns the graph's state manager.
func (g *Graph) Manager() *state.Manager {
	return g.manager
}

// AddInterruptBefore adds a node name to the interrupt-before list.
// When execution reaches this node, the graph will pause before executing it,
// create a checkpoint with pending writes, and return control to the user.
//
// This enables human-in-the-loop workflows where users can review
// and modify state before a critical node executes.
//
// Example:
//
//	g.AddInterruptBefore("send_email")
//	// Later, when resumed with WithCheckpoint() and WithResumeValue():
//	result, err := g.Run(ctx, input,
//	    WithCheckpoint(checkpoint),
//	    WithResumeValue(map[string]any{"approved": true}))
func (g *Graph) AddInterruptBefore(nodeName string) {
	g.InterruptBefore = append(g.InterruptBefore, nodeName)
}

// AddInterruptAfter adds a node name to the interrupt-after list.
// After this node executes, the graph will pause, create a checkpoint with
// pending writes (output not yet committed), and return control to the user.
//
// This enables reviewing node output before committing changes to state.
//
// Example:
//
//	g.AddInterruptAfter("generate_report")
//	// User can review the generated report in checkpoint.PendingWrites
//	// Then resume with edits:
//	result, err := g.Run(ctx, input,
//	    WithCheckpoint(checkpoint),
//	    WithResumeValue(map[string]any{"edited_report": editedContent}))
func (g *Graph) AddInterruptAfter(nodeName string) {
	g.InterruptAfter = append(g.InterruptAfter, nodeName)
}
