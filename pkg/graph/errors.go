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

	// ErrNilNode indicates a nil node was provided
	ErrNilNode = errors.New("node must not be nil")

	// ErrNilRunFunc indicates a node has a nil RunFunc
	ErrNilRunFunc = errors.New("node RunFunc must not be nil")

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

	// ErrUnreachableNode indicates a node cannot be reached from START
	ErrUnreachableNode = errors.New("unreachable node")

	// ErrSelfLoop indicates a node has an edge to itself
	ErrSelfLoop = errors.New("self-loop detected")

	// ErrHumanInterrupt indicates execution paused for human input
	ErrHumanInterrupt = errors.New("waiting for human input")

	// ErrMessageTooLarge indicates a single message exceeds the size limit
	ErrMessageTooLarge = errors.New("message size exceeds limit")

	// ErrTooManyMessages indicates the number of input messages exceeds the limit
	ErrTooManyMessages = errors.New("number of messages exceeds limit")

	// ErrTotalSizeTooLarge indicates the total size of all messages exceeds the limit
	ErrTotalSizeTooLarge = errors.New("total message size exceeds limit")
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

// NodeTimeoutError indicates a node execution exceeded its timeout.
type NodeTimeoutError struct {
	Node    string // Name of the node that timed out
	Timeout int64  // Timeout duration in milliseconds
	Cause   error  // Underlying error (usually context.DeadlineExceeded)
}

func (e *NodeTimeoutError) Error() string {
	return fmt.Sprintf("node %s exceeded timeout of %dms: %v", e.Node, e.Timeout, e.Cause)
}

func (e *NodeTimeoutError) Unwrap() error {
	return e.Cause
}

// RetryExhaustedError indicates all retry attempts for a node failed.
type RetryExhaustedError struct {
	Node     string  // Name of the node that exhausted retries
	Attempts []error // All errors from each retry attempt
}

func (e *RetryExhaustedError) Error() string {
	if len(e.Attempts) == 0 {
		return fmt.Sprintf("node %s: all retry attempts exhausted", e.Node)
	}
	return fmt.Sprintf("node %s: all %d retry attempts exhausted: %v", e.Node, len(e.Attempts), e.Attempts[len(e.Attempts)-1])
}

func (e *RetryExhaustedError) Unwrap() error {
	if len(e.Attempts) > 0 {
		return e.Attempts[len(e.Attempts)-1]
	}
	return nil
}

// MessageValidationError provides detailed information about message validation failures.
type MessageValidationError struct {
	Type          string // Type of validation error (size, count, total)
	Limit         int    // The configured limit
	Actual        int    // The actual value that exceeded the limit
	MessageIndex  int    // Index of the problematic message (for single message errors, -1 for aggregate)
	UnderlyingErr error  // The sentinel error (ErrMessageTooLarge, etc.)
}

func (e *MessageValidationError) Error() string {
	switch e.Type {
	case "message_size":
		return fmt.Sprintf("message at index %d exceeds size limit: %d bytes > %d bytes limit",
			e.MessageIndex, e.Actual, e.Limit)
	case "message_count":
		return fmt.Sprintf("too many messages: %d messages > %d limit", e.Actual, e.Limit)
	case "total_size":
		return fmt.Sprintf("total message size exceeds limit: %d bytes > %d bytes limit", e.Actual, e.Limit)
	default:
		return fmt.Sprintf("message validation failed: %v", e.UnderlyingErr)
	}
}

func (e *MessageValidationError) Unwrap() error {
	return e.UnderlyingErr
}
