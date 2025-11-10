package graph

import (
	"context"
	"slices"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// PregelExecutor implements the Executor interface using the Pregel BSP execution model.
// It delegates to Compiled's internal execution logic while providing a clean interface.
type PregelExecutor struct {
	cg *Compiled
}

// NewPregelExecutor creates a new Pregel-based executor for the given Compiled.
func NewPregelExecutor(cg *Compiled) *PregelExecutor {
	return &PregelExecutor{
		cg: cg,
	}
}

// Execute runs the graph to completion and returns the final result.
func (e *PregelExecutor) Execute(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (*InvokeResult, error) {
	// Convert ExecuteOptions to internal runOptions
	runOpts := runOptions{
		maxIterations:      options.MaxIterations,
		maxConcurrency:     options.MaxWorkers,
		checkpointInterval: options.CheckpointInterval,
		runID:              options.RunID,
	}

	// Use existing invokeWithOptions implementation
	messages, err := e.cg.invokeWithOptions(ctx, initialMessages, runOpts)
	if err != nil {
		return nil, err
	}

	// Build result
	result := &InvokeResult{
		Messages: messages,
		State:    e.cg.stateManager.GetAll(),
	}

	return result, nil
}

// Stream executes the graph with real-time event streaming.
func (e *PregelExecutor) Stream(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (<-chan interface{}, <-chan error) {
	eventChan := make(chan any, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)

		// Convert ExecuteOptions to internal runOptions
		runOpts := runOptions{
			maxIterations:      options.MaxIterations,
			maxConcurrency:     options.MaxWorkers,
			checkpointInterval: options.CheckpointInterval,
			runID:              options.RunID,
		}

		// Use existing streamWithOptions implementation
		stream, err := e.cg.streamWithOptions(ctx, initialMessages, runOpts)
		if err != nil {
			errChan <- err
			return
		}
		defer stream.Cancel()

		// Forward StreamEvents as interface{}
		for stream.Next() {
			event := stream.Current()
			if event.Err != nil {
				errChan <- event.Err
				return
			}
			eventChan <- event
		}

		if err := stream.Err(); err != nil {
			errChan <- err
		}
	}()

	return eventChan, errChan
}

// Pause pauses execution before the specified node.
func (e *PregelExecutor) Pause(nodeName string) {
	// Delegate to Compiled's pause mechanism
	e.cg.markPaused(nodeName)
}

// Resume resumes execution of a paused node.
func (e *PregelExecutor) Resume(nodeName string) {
	// Delegate to Compiled's resume mechanism
	e.cg.clearPaused(nodeName)
}

// IsPaused returns whether the specified node is currently paused.
func (e *PregelExecutor) IsPaused(nodeName string) bool {
	// Check if node is paused via Compiled runtime state
	e.cg.runtimeMu.RLock()
	defer e.cg.runtimeMu.RUnlock()
	if e.cg.runtime == nil {
		return false
	}
	// Check if node is in the paused list
	pausedNodes := e.cg.runtime.pausedNames()
	return slices.Contains(pausedNodes, nodeName)
}

// CurrentSuperstep returns the current superstep number.
func (e *PregelExecutor) CurrentSuperstep() int64 {
	return e.cg.CurrentSuperstep()
}

// Stats returns execution statistics.
func (e *PregelExecutor) Stats() ExecutionStats {
	// Return basic stats based on Compiled state
	return ExecutionStats{
		Supersteps: e.cg.CurrentSuperstep(),
	}
}

// Verify PregelExecutor implements Executor interface.
var _ Executor = (*PregelExecutor)(nil)
