package core

import (
	"context"
)

// RunOptions holds per-run configuration flags passed to Runner.Run.
// This mirrors runner.RunOptions but is defined here so callers can depend on the core API.
type RunOptions struct {
	// MaxModelCalls limits the number of model calls per run.
	MaxModelCalls int

	// StateDelta holds optional state updates to merge into the session state.
	StateDelta map[string]any

	// SaveInputBlobsAsArtifacts determines whether to save input blobs as artifacts.
	SaveInputBlobsAsArtifacts bool
}

// RunResult represents an event or an error produced while executing a run.
// The results channel returned from Runner.Run will carry a stream of these.
type RunResult struct {
	RunID string
	Event *Event
	Err   error
}

// Runner abstracts an agent execution engine capable of streaming events for a run.
// Implementations should be safe for concurrent use.
type Runner interface {
	// Run starts an asynchronous invocation.
	// Returns the run ID, a channel of results, or an error if it failed to start.
	Run(
		ctx context.Context,
		userID, sessionID string,
		userParts []Part,
		optFns ...func(o *RunOptions),
	) (string, <-chan RunResult, error)

	// Cancel cancels a running run by ID.
	Cancel(runID string) error

	// Close waits for active runs to finish and releases resources.
	Close() error
}
