package viz

import (
	"context"
	"fmt"
	"sync"
)

// RunController manages a single graph execution run.
// It provides control over pausing, stepping, and canceling executions.
type RunController struct {
	runID    string
	runnable Runnable
	ctx      context.Context
	cancel   context.CancelFunc

	mu     sync.RWMutex
	status RunStatus
}

// RunStatus represents the execution state.
type RunStatus string

// Run status constants define the possible states of a graph execution.
const (
	StatusPending   RunStatus = "pending"   // Run has not yet started
	StatusRunning   RunStatus = "running"   // Run is currently executing
	StatusPaused    RunStatus = "paused"    // Run is paused (for human-in-loop)
	StatusCompleted RunStatus = "completed" // Run finished successfully
	StatusFailed    RunStatus = "failed"    // Run failed with an error
	StatusCanceled  RunStatus = "canceled"  // Run was canceled by user
)

// ErrInvalidState is returned when an operation is invalid for the current state.
var ErrInvalidState = fmt.Errorf("invalid state for operation")

// NewRunController creates a new run controller.
func NewRunController(runID string, runnable Runnable) *RunController {
	ctx, cancel := context.WithCancel(context.Background())
	return &RunController{
		runID:    runID,
		runnable: runnable,
		ctx:      ctx,
		cancel:   cancel,
		status:   StatusPending,
	}
}

// Context returns the controller's context for cancellation propagation.
func (rc *RunController) Context() context.Context {
	return rc.ctx
}

// Cancel stops the execution.
func (rc *RunController) Cancel() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.cancel != nil {
		rc.cancel()
	}
	rc.status = StatusCanceled
}

// Status returns the current execution status.
func (rc *RunController) Status() RunStatus {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.status
}

// SetStatus updates the execution status.
func (rc *RunController) SetStatus(status RunStatus) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.status = status
}
