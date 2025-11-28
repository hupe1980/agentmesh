package graph

import (
	"errors"
	"strconv"
)

// Sentinel errors for the graph package.
// These errors can be used with errors.Is() for error checking.
// All errors follow the pattern: "graph: <category>"
var (
	// ErrStateApply is returned when state updates cannot be applied.
	ErrStateApply = errors.New("graph: state apply failed")

	// ErrSnapshotCreate is returned when creating a state snapshot fails.
	ErrSnapshotCreate = errors.New("graph: snapshot creation failed")

	// ErrCheckpointLoad is returned when loading a checkpoint fails.
	ErrCheckpointLoad = errors.New("graph: checkpoint load failed")

	// ErrCheckpointSave is returned when saving a checkpoint fails.
	ErrCheckpointSave = errors.New("graph: checkpoint save failed")

	// ErrNodeNotFound is returned when a node cannot be found in the graph.
	ErrNodeNotFound = errors.New("graph: node not found")

	// ErrExecutorNil is returned when a nil executor is provided.
	ErrExecutorNil = errors.New("graph: executor cannot be nil")

	// ErrBuilderError is returned when the builder encounters an error during construction.
	ErrBuilderError = errors.New("graph: builder error")

	// ErrRunIDRequired is returned when checkpointing is enabled without a run ID.
	ErrRunIDRequired = errors.New("graph: run ID required for checkpointing")

	// ErrRetryExceeded is returned when max retry attempts are exceeded.
	ErrRetryExceeded = errors.New("graph: max retry attempts exceeded")

	// ErrDistributedState is returned when distributed state synchronization fails.
	ErrDistributedState = errors.New("graph: distributed state sync failed")

	// ErrRoutingTargets is returned when a node fails to specify routing targets.
	ErrRoutingTargets = errors.New("graph: node must specify routing targets")

	// ErrNamespaceViolation is returned when a namespaced node attempts to update keys outside its namespace.
	ErrNamespaceViolation = errors.New("graph: namespace access violation")

	// ErrSubgraphExecution is returned when a subgraph execution fails.
	ErrSubgraphExecution = errors.New("graph: subgraph execution failed")

	// ErrInputMapping is returned when subgraph input mapping fails.
	ErrInputMapping = errors.New("graph: input mapping failed")

	// ErrOutputMapping is returned when subgraph output mapping fails.
	ErrOutputMapping = errors.New("graph: output mapping failed")

	// ErrEntryPointDuplicate is returned when an entry point is added twice.
	ErrEntryPointDuplicate = errors.New("graph: entry point already exists")

	// ErrNodeConfigInvalid is returned when a node configuration is invalid.
	ErrNodeConfigInvalid = errors.New("graph: invalid node config")

	// ErrValidation is returned when graph validation fails.
	ErrValidation = errors.New("graph: validation failed")
)

// retryExceededError wraps retry exceeded failures with context and preserves error chain.
type retryExceededError struct {
	sentinel    error
	maxAttempts int
	lastErr     error
}

// Error implements the error interface.
func (e *retryExceededError) Error() string {
	return "graph: max retry attempts exceeded: max attempts (" + strconv.Itoa(e.maxAttempts) + "): " + e.lastErr.Error()
}

// Unwrap returns the wrapped error for errors.Is/As compatibility.
// Returns the last error so errors.Is can match the underlying cause.
func (e *retryExceededError) Unwrap() error {
	return e.lastErr
}

// Is checks if the target error is ErrRetryExceeded.
func (e *retryExceededError) Is(target error) bool {
	return target == ErrRetryExceeded
}
