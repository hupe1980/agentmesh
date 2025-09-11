package core

import "errors"

// ErrAgentNotFound indicates that a named agent was not found in the hierarchy.
var ErrAgentNotFound = errors.New("agent not found")

// ErrSessionNotFound indicates a session ID does not exist in the session store.
var ErrSessionNotFound = errors.New("session not found")

// ErrMaxModelCallsExceeded is returned when the model call limiter threshold
// is exceeded for a single run.
var ErrMaxModelCallsExceeded = errors.New("max model calls exceeded")

// ErrMemoryNotFound indicates a requested memory item or session bucket was not found.
var ErrMemoryNotFound = errors.New("memory not found")

// ErrToolNotFound indicates the requested tool/function does not exist.
var ErrToolNotFound = errors.New("tool not found")

// ErrInvalidToolArgs indicates the provided tool arguments failed validation or could not be parsed.
var ErrInvalidToolArgs = errors.New("invalid tool arguments")

// ErrRunNotFound indicates a run ID is unknown to the runner.
var ErrRunNotFound = errors.New("run not found")

// ErrParallelTimeout indicates a parallel agent run exceeded its configured timeout.
var ErrParallelTimeout = errors.New("parallel execution timed out")

// ErrArtifactNotFound indicates a requested artifact was not found for a session/key.
var ErrArtifactNotFound = errors.New("artifact not found")
