package pregel

import "errors"

// Sentinel errors for the pregel package
var (
	// ErrGraphRequired indicates a nil graph was provided to the runtime
	ErrGraphRequired = errors.New("pregel: graph must not be nil")

	// ErrUnknownNode indicates a vertex name has no corresponding node implementation
	ErrUnknownNode = errors.New("pregel: unknown node")

	// ErrAggregatorsNotConfigured indicates aggregators were not set up but aggregation was attempted
	ErrAggregatorsNotConfigured = errors.New("pregel: aggregators not configured")

	// ErrUnknownAggregator indicates an unregistered aggregator name was used
	ErrUnknownAggregator = errors.New("pregel: unknown aggregator")

	// ErrNodePanicked indicates a node panicked during execution
	ErrNodePanicked = errors.New("pregel: node panicked")

	// ErrMaxIterationsExceeded indicates the computation exceeded the maximum allowed iterations
	ErrMaxIterationsExceeded = errors.New("pregel: maximum iterations exceeded")

	// ErrMailboxFull indicates a vertex's mailbox has reached its maximum size
	ErrMailboxFull = errors.New("pregel: mailbox full - message limit exceeded")
)
