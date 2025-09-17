package core

import "context"

// Flow is implemented by orchestration flows.
// A Flow processes the initial request, streams model output, optionally
// handles function calls, and may trigger agent transfers.
type Flow interface {
	Execute(ctx context.Context, reqCtx RequestContext, queue EventWriter) error
}

// FlowSelector chooses the appropriate Flow for a given FlowAgent.
type FlowSelector interface {
	Select(agent FlowAgent) Flow
}
