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
	onEvent func(ctx context.Context, ev *core.Event) (*core.Event, error)
}

func (q *sessionWriter) Write(ctx context.Context, ev *core.Event) error {
	log := logging.FromContext(ctx).With("session_id", q.session.ID)

	// Persist the event to the session store (always, even if context canceled)
	if err := q.store.AppendEvent(ctx, q.session, ev); err != nil {
		return fmt.Errorf("failed to append event to session: %w", err)
	}

	// Run OnEvent plugin hook to allow modifications or side-effects
	if q.onEvent != nil {
		if repl, err := q.onEvent(ctx, ev); err != nil {
			return fmt.Errorf("plugin: on_event: %w", err)
		} else if repl != nil {
			ev = repl
		}
	}

	// Fast-exit if the context is already canceled (avoid blocking on channel send)
	if err := ctx.Err(); err != nil {
		log.Debug("runner writer context canceled, skipping event forward", "event_id", ev.ID, "error", err)
		return err
	}

	// Forward the event to the results channel
	select {
	case <-ctx.Done():
		return ctx.Err() // respects cancellation
	case q.results <- core.RunResult{RunID: q.runID, Event: ev}:
		log.Debug("runner delivered event", "event_id", ev.ID)

		return nil
	}
}
