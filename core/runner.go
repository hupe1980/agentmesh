package core

import "context"

// Runner defines the minimal orchestration contract for executing a root agent
// within a conversational session. It provides:
//   - Asynchronous execution via Run (streaming events + terminal error channel)
//   - Cooperative cancellation through Cancel
//   - Stable run identifiers for tracking / external control
//
// Semantics & Guarantees:
//   - Event Ordering: Events emitted within a single run are delivered
//     in the order produced by the underlying agent pipeline.
//   - Channel Lifecycle: The returned events channel is closed after the
//     run completes (success, error, or cancellation). The error channel
//     carries at most one terminal error then closes (buffered size 1).
//   - Cancellation: Context cancellation or explicit Cancel(runID)
//     stops further event emission and triggers cleanup.
//   - Partial Events: Implementations MAY emit partial events; consumers should
//     rely on IsPartial() to decide persistence or display strategy.
type Runner interface {
	// Run initiates an asynchronous agent execution bound to sessionID using the
	// provided userContent as the starting input. It returns:
	//   runID - stable identifier for cancellation / tracking
	//   eventsCh     - ordered stream of events (closed on completion)
	//   errorsCh     - terminal error channel (size 1, closed after send/none)
	// The immediate error return covers startup failures (e.g. session load).
	Run(ctx context.Context, sessionID string, userContent Content) (string, <-chan Event, <-chan error, error)

	// Cancel requests cooperative termination of an in‑flight run.
	// It MUST be idempotent; cancelling an unknown or already finished
	// invocation returns an error describing the condition.
	Cancel(runID string) error
}
