package core

import "context"

// Engine coordinates agent execution and event emission.
//
// A concrete implementation is responsible for:
//   - Registering available agents (by name) via Register
//   - Spawning asynchronous runs (RunAsync) returning event + error channels
//   - Synchronous convenience execution (Run) collecting emitted events
//
// Implementations SHOULD:
//   - Guarantee ordering of events per invocation where necessary
//   - Propagate context cancellation to underlying agent Run calls
//   - Close returned channels when an async run terminates
//   - Surface terminal errors via the error channel (async) or direct return (sync)
type Engine interface {
	// Register makes an agent available for later invocation by name.
	Register(a Agent)

	// Invoke starts an asynchronous agent invocation returning streaming event
	// and terminal error channels. Channels are closed when execution completes
	// or the context is cancelled. This is the primary API; prefer it for
	// streaming / interactive consumption.
	//
	// Returns:
	//   - invocationID: unique identifier for this invocation (for cancellation / tracking)
	//   - eventsCh: streamed events
	//   - errorsCh: terminal error channel (buffered size 1)
	//   - err: immediate error starting invocation
	Invoke(
		ctx context.Context,
		sessionID, agentName string,
		userContent Content,
	) (string, <-chan Event, <-chan error, error)

	// InvokeSync executes an agent to completion, collecting all emitted
	// events into a slice. Convenience wrapper that drains Invoke.
	// InvokeSync returns collected events and the invocationID that produced them.
	InvokeSync(ctx context.Context, sessionID, agentName string, userContent Content) (string, []Event, error)
}
