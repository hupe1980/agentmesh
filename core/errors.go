package core

import (
	"errors"
	"fmt"
)

// Core and hierarchy errors (grouped for consistent style)
var (
	// Sessions
	ErrSessionNotFound = errors.New("session not found")

	// Limiting / execution
	ErrMaxModelCallsExceeded = errors.New("max model calls exceeded")
	ErrParallelTimeout       = errors.New("parallel execution timed out")

	// Memory & artifacts
	ErrMemoryNotFound   = errors.New("memory not found")
	ErrArtifactNotFound = errors.New("artifact not found")

	// Tools
	ErrToolNotFound    = errors.New("tool not found")
	ErrInvalidToolArgs = errors.New("invalid tool arguments")

	// Runs
	ErrRunNotFound     = errors.New("run not found")
	ErrNoFinalResponse = errors.New("no final response emitted")

	// Hierarchy management
	ErrAgentNotFound         = errors.New("agent not found")
	ErrParentAlreadySet      = errors.New("agent: parent already set")
	ErrSelfParent            = errors.New("agent: cannot set parent to self")
	ErrParentCycle           = errors.New("agent: setting parent would create a cycle")
	ErrParentNil             = errors.New("agent: parent cannot be nil")
	ErrSubAgentSelf          = errors.New("agent: cannot add self as sub-agent")
	ErrSubAgentAlreadyExists = errors.New("agent: sub-agent already exists")
)

// ErrKeyMissing is returned when a key does not exist in the state snapshot.
type ErrKeyMissing struct {
	Key string
}

func (e ErrKeyMissing) Error() string {
	return fmt.Sprintf("state: key %q not found", e.Key)
}

// ErrTypeMismatch is returned when a value is of unexpected type.
type ErrTypeMismatch struct {
	Key        string
	Expected   string
	ActualType string
}

func (e ErrTypeMismatch) Error() string {
	return fmt.Sprintf("state: key %q has type %s, expected %s", e.Key, e.ActualType, e.Expected)
}
