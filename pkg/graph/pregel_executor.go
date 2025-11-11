package graph

import (
	"context"
	"iter"
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

// Execute runs the graph and returns an iterator of execution events.
// This method directly delegates to Compiled.Run() with the appropriate options.
func (e *PregelExecutor) Execute(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) iter.Seq2[Event, error] {
	// Convert ExecuteOptions to RunOption functions
	var optFns []RunOption
	if options.MaxIterations > 0 {
		optFns = append(optFns, WithMaxIterations(options.MaxIterations))
	}
	if options.MaxWorkers > 0 {
		optFns = append(optFns, WithMaxConcurrency(options.MaxWorkers))
	}
	if options.RunID != "" {
		optFns = append(optFns, WithRunID(options.RunID))
	}
	// Note: CheckpointInterval is not directly supported here as PregelExecutor
	// does not manage a Checkpointer instance. Pass it via graph options if needed.

	// Return the iterator directly from Compiled.Run()
	return e.cg.Run(ctx, initialMessages, optFns...)
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
