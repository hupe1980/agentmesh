package graph

import (
	"errors"
	"fmt"
)

// Sentinel errors for the graph package
var (
	// ErrNodeNotFound indicates a requested node does not exist in the graph
	ErrNodeNotFound = errors.New("node not found")

	// ErrNodeAlreadyExists indicates a node with the same name is already registered
	ErrNodeAlreadyExists = errors.New("node already exists")

	// ErrInvalidNodeName indicates an empty or invalid node name
	ErrInvalidNodeName = errors.New("invalid node name")

	// ErrCyclicGraph indicates the graph contains a cycle
	ErrCyclicGraph = errors.New("cyclic graph detected")

	// ErrInvalidEdge indicates an edge references non-existent nodes
	ErrInvalidEdge = errors.New("invalid edge")

	// ErrNilGraph indicates a nil graph was provided
	ErrNilGraph = errors.New("graph must not be nil")

	// ErrNilContext indicates a nil context was provided
	ErrNilContext = errors.New("context must not be nil")

	// ErrAggregatorsNotConfigured indicates aggregators were not set up
	ErrAggregatorsNotConfigured = errors.New("aggregators not configured")

	// ErrUnknownAggregator indicates an unregistered aggregator name was used
	ErrUnknownAggregator = errors.New("unknown aggregator")

	// ErrCheckpointNotFound indicates a requested checkpoint does not exist
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	// ErrInvalidState indicates state is nil or invalid
	ErrInvalidState = errors.New("invalid state")

	// ErrMaxIterationsExceeded indicates the graph execution exceeded the maximum allowed iterations
	ErrMaxIterationsExceeded = errors.New("max iterations exceeded")
)

// NodeExecutionError wraps errors that occur during node execution,
// providing context about which node failed and at what superstep.
type NodeExecutionError struct {
	Node      string // Name of the node that failed
	Superstep int64  // Superstep at which the failure occurred
	Cause     error  // Underlying error
}

func (e *NodeExecutionError) Error() string {
	if e.Superstep >= 0 {
		return fmt.Sprintf("node %s failed at superstep %d: %v", e.Node, e.Superstep, e.Cause)
	}
	return fmt.Sprintf("node %s failed: %v", e.Node, e.Cause)
}

func (e *NodeExecutionError) Unwrap() error {
	return e.Cause
}

// ValidationError indicates a graph validation failure with specific context.
type ValidationError struct {
	Message string // Human-readable error message
	Field   string // Field or component that failed validation (optional)
	Value   string // Invalid value (optional)
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return "validation error: " + e.Field + ": " + e.Message
	}
	return "validation error: " + e.Message
}

// MessageLimitError indicates message retention limit was configured incorrectly.
type MessageLimitError struct {
	Limit     int
	Attempted int
}

func (e *MessageLimitError) Error() string {
	return fmt.Sprintf("message limit exceeded: limit=%d, attempted=%d", e.Limit, e.Attempted)
}
