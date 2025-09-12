package runner

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
)

// sessionWriter implements core.EventWriter and is responsible for persisting
// non-partial events to the session store and forwarding all events to results.
type sessionWriter struct {
	runID   string
	session *core.Session
	store   core.SessionStore
	results chan<- core.RunResult
}

func (q *sessionWriter) Write(ctx context.Context, ev *core.Event) error {
	if err := q.store.AppendEvent(ctx, q.session, ev); err != nil {
		return fmt.Errorf("failed to append event to session: %w", err)
	}

	// Fast-path: if already canceled, return context error (avoid random select choice).
	if err := ctx.Err(); err != nil {
		return err
	}

	// Forward the event to consumers (still respect cancellation during send)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.results <- core.RunResult{RunID: q.runID, Event: ev}:
		logging.FromContext(ctx).Debug(
			"runner delivered event",
			"session_id", q.session.ID,
			"event_id", ev.ID,
		)
		return nil
	}
}
