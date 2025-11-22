package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Graph represents a mutable computational graph with nodes and edges.
type Graph struct {
	Nodes           map[string]Node
	Edges           []Edge
	Branches        []ConditionalEdges
	NodeConfigs     map[string]*NodeConfig // Execution policies per node
	InterruptBefore []string               // Nodes to interrupt before execution
	InterruptAfter  []string               // Nodes to interrupt after execution
	manager         *state.Manager
}

// NewGraph creates a new graph with the given state manager.
func NewGraph(manager *state.Manager) (*Graph, error) {
	if manager == nil {
		return nil, fmt.Errorf("manager cannot be nil")
	}
	return &Graph{
		Nodes:           make(map[string]Node),
		Edges:           make([]Edge, 0),
		Branches:        make([]ConditionalEdges, 0),
		NodeConfigs:     make(map[string]*NodeConfig),
		InterruptBefore: make([]string, 0), // Initialize interrupt lists
		InterruptAfter:  make([]string, 0),
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
//
// Example with cache policy:
//
//	g.AddNode(expensiveNode, WithCachePolicy(&CachePolicy{
//	    Enabled: true,
//	    TTL: 5 * time.Minute,
//	    KeyFunc: func(ctx context.Context, state map[string]any) string {
//	        return fmt.Sprintf("key:%v", state["input"])
//	    },
//	}))
func (g *Graph) AddNode(n Node, opts ...NodeOption) error {
	if n == nil {
		return fmt.Errorf("node cannot be nil")
	}
	if n.Name() == "" {
		return fmt.Errorf("node name cannot be empty")
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

// SetNodeRetryPolicy sets or updates the retry policy for an existing node.
// Returns an error if the node doesn't exist or doesn't support retry.
func (g *Graph) SetNodeRetryPolicy(name string, retryPolicy *RetryPolicy) error {
	node, exists := g.Nodes[name]
	if !exists {
		return fmt.Errorf("node not found: %s", name)
	}

	// Only BaseNode supports setting retry policy after creation
	baseNode, ok := node.(*BaseNode)
	if !ok {
		return fmt.Errorf("node %s does not support setting retry policy", name)
	}

	baseNode.retryPolicy = retryPolicy
	return nil
}

// AddEdge adds a directed edge between two nodes.
func (g *Graph) AddEdge(from, to string) {
	g.Edges = append(g.Edges, Edge{From: from, To: to})
}

// AddConditionalEdges adds conditional routing based on runtime state.
// The condition function receives a ReadView and returns target node names.
func (g *Graph) AddConditionalEdges(from string, condition func(context.Context, *state.ReadView) []string, targets []string) {
	g.Branches = append(g.Branches, ConditionalEdges{
		From:      from,
		Condition: condition,
		Targets:   targets,
	})
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

// Shutdown gracefully shuts down the graph and its plugins.
// Call this when you're done using the graph to clean up resources.
//
// Example:
//
//	defer g.Shutdown(context.Background())
func (g *Graph) Shutdown(ctx context.Context) error {
	// No shutdown needed for graph itself
	return nil
}
