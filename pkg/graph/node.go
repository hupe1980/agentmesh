package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// NodeRunnable represents a graph node that can execute logic.
// This interface allows custom node implementations beyond the standard Node type.
// Note: This is different from the generic Runnable[I, O] interface which is
// used for composable agents and graphs.
type NodeRunnable interface {
	Run(ctx context.Context, s state.Reader) (*NodeResult, error)
}

// NodeResult contains the output of a node execution.
// Updates modify graph state values, and Messages append to the conversation history.
// The framework automatically wraps Messages in state.ExecutionResult with execution metadata.
type NodeResult struct {
	Updates  map[string]any    // State updates (key-value pairs)
	Messages []message.Message // Messages to append (framework adds metadata)
}

// RetryPolicy configures automatic retry behavior for transient failures.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of execution attempts (including the initial one).
	// A value <= 1 means no retries (single attempt only).
	MaxAttempts int

	// Backoff computes the wait duration before the nth retry attempt.
	// If nil, exponential backoff (2^n seconds) is used.
	Backoff func(attempt int) time.Duration

	// Retryable determines if an error should trigger a retry.
	// If nil, all errors are considered retryable.
	Retryable func(error) bool
}

// DefaultBackoff returns an exponential backoff strategy: 2^attempt seconds.
func DefaultBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	base := time.Second
	for i := 1; i < attempt; i++ {
		base *= 2
	}
	return base
}

// Node represents a vertex in the execution graph with custom logic.
// Each node has a unique name and a RunFunc that performs computation.
type Node struct {
	Name        string // Unique identifier for the node
	RunFunc     func(ctx context.Context, s state.Writer) (*NodeResult, error)
	RetryPolicy *RetryPolicy // Optional retry configuration
}

// Run invokes the configured RunFunc for the node.
func (n *Node) Run(ctx context.Context, s state.Writer) (*NodeResult, error) {
	if n.RunFunc == nil {
		return nil, fmt.Errorf("node %q: %w: no Run function", n.Name, ErrNodeNotFound)
	}
	return n.RunFunc(ctx, s)
}

// Edge represents a directed connection between two nodes.
type Edge struct {
	From string // Source node name
	To   string // Destination node name
}

// ConditionalEdges represents dynamic routing based on node output.
// The Condition function evaluates the graph state to determine which target nodes to execute.
type ConditionalEdges struct {
	From      string                                       // Source node name
	Targets   []string                                     // Possible target node names
	Condition func(context.Context, state.Reader) []string // Function determining which targets to activate
}
