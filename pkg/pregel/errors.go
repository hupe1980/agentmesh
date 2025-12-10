package pregel

import "errors"

// Sentinel errors for the pregel package
var (
	// ErrGraphRequired indicates a nil graph was provided to the runtime
	ErrGraphRequired = errors.New("pregel: graph must not be nil")

	// ErrNilVertex indicates a nil vertex was provided
	ErrNilVertex = errors.New("pregel: vertex must not be nil")

	// ErrUnknownVertex indicates a vertex name has no corresponding vertex implementation
	ErrUnknownVertex = errors.New("pregel: unknown vertex")

	// ErrAggregatorsNotConfigured indicates aggregators were not set up but aggregation was attempted
	ErrAggregatorsNotConfigured = errors.New("pregel: aggregators not configured")

	// ErrUnknownAggregator indicates an unregistered aggregator name was used
	ErrUnknownAggregator = errors.New("pregel: unknown aggregator")

	// ErrVertexPanicked indicates a vertex panicked during execution
	ErrVertexPanicked = errors.New("pregel: vertex panicked")

	// ErrMaxIterationsExceeded indicates the computation exceeded the maximum allowed iterations
	ErrMaxIterationsExceeded = errors.New("pregel: maximum iterations exceeded")

	// ErrMailboxFull indicates a vertex's mailbox has reached its maximum size
	ErrMailboxFull = errors.New("pregel: mailbox full - message limit exceeded")

	// ErrMessageBusClosed indicates that the message bus has been closed
	ErrMessageBusClosed = errors.New("pregel: message bus is closed")

	// ErrTLSRequired indicates TLS is required for authenticated connections
	ErrTLSRequired = errors.New("pregel: TLS required when using password authentication")

	// ErrShutdownTimeout indicates the worker pool did not shut down within the timeout
	ErrShutdownTimeout = errors.New("pregel: worker pool shutdown timeout")
)
