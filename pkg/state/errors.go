package state

import "errors"

// Sentinel errors for state management
var (
	// ErrAggregatorsNotConfigured indicates aggregators were not set up
	ErrAggregatorsNotConfigured = errors.New("aggregators not configured")

	// ErrUnknownAggregator indicates an unregistered aggregator name was used
	ErrUnknownAggregator = errors.New("unknown aggregator")

	// ErrCheckpointNotFound indicates a requested checkpoint does not exist
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	// ErrInvalidState indicates state is nil or invalid
	ErrInvalidState = errors.New("invalid state")

	// ErrNodeExecution indicates a node execution failed
	// Use errors.Is(err, ErrNodeExecution) to check for node-level failures
	ErrNodeExecution = errors.New("node execution failed")
)
