// Package viz provides sentinel and structured errors for the viz package.
package viz

import (
	"errors"
	"fmt"
)

// =============================================================================
// Sentinel Errors
// =============================================================================

var (
	// ErrInvalidState is returned when an operation is invalid for the current state.
	ErrInvalidState = errors.New("viz: invalid state for operation")

	// ErrExecutionStopped is returned when execution is stopped by user.
	ErrExecutionStopped = errors.New("viz: execution stopped by user")

	// ErrCommandQueueFull is returned when the command queue is full.
	ErrCommandQueueFull = errors.New("viz: command queue is full")

	// ErrExecutionTimeout is returned when execution times out.
	ErrExecutionTimeout = errors.New("viz: execution timeout")

	// ErrFirstCheckpoint is returned when already at first checkpoint.
	ErrFirstCheckpoint = errors.New("viz: already at first checkpoint")

	// ErrLastCheckpoint is returned when already at last checkpoint.
	ErrLastCheckpoint = errors.New("viz: already at last checkpoint")

	// ErrNilCheckpoint is returned when a nil checkpoint is provided.
	ErrNilCheckpoint = errors.New("viz: cannot compute diff: nil checkpoint provided")
)

// =============================================================================
// Structured Errors
// =============================================================================

// RunnableNotFoundError represents an error when a runnable is not found.
// Use errors.As to extract the ID field for programmatic handling.
type RunnableNotFoundError struct {
	// ID is the runnable ID that was not found.
	ID string
}

// Error implements the error interface.
func (e *RunnableNotFoundError) Error() string {
	return fmt.Sprintf("viz: runnable not found: %s", e.ID)
}

// Is enables comparison with sentinel errors.
func (e *RunnableNotFoundError) Is(target error) bool {
	_, ok := target.(*RunnableNotFoundError)
	return ok
}

// RunnableAlreadyRegisteredError represents an error when a runnable is already registered.
// Use errors.As to extract the ID field for programmatic handling.
type RunnableAlreadyRegisteredError struct {
	// ID is the runnable ID that was already registered.
	ID string
}

// Error implements the error interface.
func (e *RunnableAlreadyRegisteredError) Error() string {
	return fmt.Sprintf("viz: runnable already registered: %s", e.ID)
}

// Is enables comparison with sentinel errors.
func (e *RunnableAlreadyRegisteredError) Is(target error) bool {
	_, ok := target.(*RunnableAlreadyRegisteredError)
	return ok
}

// BreakpointNotFoundError represents an error when a breakpoint is not found.
// Use errors.As to extract the ID field for programmatic handling.
type BreakpointNotFoundError struct {
	// ID is the breakpoint ID that was not found.
	ID string
}

// Error implements the error interface.
func (e *BreakpointNotFoundError) Error() string {
	return fmt.Sprintf("viz: breakpoint not found: %s", e.ID)
}

// Is enables comparison with sentinel errors.
func (e *BreakpointNotFoundError) Is(target error) bool {
	_, ok := target.(*BreakpointNotFoundError)
	return ok
}

// InvalidCommandError represents an error when a command is invalid for the current state.
// Use errors.As to extract the Command and State fields for programmatic handling.
type InvalidCommandError struct {
	// Command is the command that was attempted.
	Command ExecutionCommand
	// State is the current execution state.
	State ExecutionState
}

// Error implements the error interface.
func (e *InvalidCommandError) Error() string {
	return fmt.Sprintf("viz: cannot send command %s: execution is %s", e.Command, e.State)
}

// Is enables comparison with sentinel errors.
func (e *InvalidCommandError) Is(target error) bool {
	_, ok := target.(*InvalidCommandError)
	return ok
}
