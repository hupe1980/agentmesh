package runner

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// RunFinal executes Runner.Run and blocks until completion, returning the final response event.
// It returns the runID immediately and either the final event or an error.
// If an error occurs during streaming, the run is canceled before returning.
func (r *Runner) RunFinal(
	ctx context.Context,
	userID, sessionID string,
	userParts []core.Part,
	optFns ...func(o *core.RunOptions),
) (string, *core.Event, error) {
	runID, results, err := r.Run(ctx, userID, sessionID, userParts, optFns...)
	if err != nil {
		return "", nil, err
	}

	var final *core.Event

	for {
		select {
		case <-ctx.Done():
			// Best-effort cancel and return context error
			_ = r.Cancel(runID)
			return runID, nil, ctx.Err()
		case res, ok := <-results:
			if !ok {
				if final == nil {
					return runID, nil, core.ErrNoFinalResponse
				}
				return runID, final, nil
			}

			if res.Err != nil {
				_ = r.Cancel(runID)
				return runID, nil, res.Err
			}

			if res.Event == nil {
				continue
			}

			// Prefer the first IsFinalResponse; fall back to the last event if none marked final
			if res.Event.IsFinalResponse() {
				final = res.Event
				// Drain remaining results quickly until channel closes or context canceled
				// to avoid leaking the producer, but return immediately for responsiveness.
				return runID, final, nil
			}

			// Keep the latest non-final as a fallback
			final = res.Event
		}
	}
}

// RunFinalText is like RunFinal but returns only the concatenated text from the final event.
func (r *Runner) RunFinalText(
	ctx context.Context,
	userID, sessionID string,
	userParts []core.Part,
	optFns ...func(o *core.RunOptions),
) (string, string, error) {
	runID, ev, err := r.RunFinal(ctx, userID, sessionID, userParts, optFns...)
	if err != nil {
		return runID, "", err
	}

	if ev == nil {
		return runID, "", core.ErrNoFinalResponse
	}

	return runID, ev.Text(), nil
}
