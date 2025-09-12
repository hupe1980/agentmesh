package runner

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionWriter_Success ensures that a normal event write persists the
// event into the session and forwards it to the results channel.
func TestSessionWriter_Success(t *testing.T) {
	store := session.NewInMemoryStore()
	ctx := context.Background()
	sess, err := store.GetOrCreate(ctx, "app", "user", "sess1")
	require.NoError(t, err)

	results := make(chan core.RunResult, 1)
	w := &sessionWriter{runID: "run1", session: sess, store: store, results: results}

	ev := core.NewUserContentEvent("run1", &core.TextPart{Text: "hello"})
	require.NoError(t, w.Write(ctx, ev))

	// Session passed to writer should be mutated with event.
	assert.Len(t, sess.Events(), 1)
	assert.Equal(t, ev.ID, sess.Events()[0].ID)

	select {
	case rr := <-results:
		assert.Equal(t, "run1", rr.RunID)
		assert.Equal(t, ev, rr.Event)
	default:
		assert.Fail(t, "expected result forwarded")
	}
}

// TestSessionWriter_StoreError ensures store append errors propagate and no result is sent.
func TestSessionWriter_StoreError(t *testing.T) {
	store := session.NewInMemoryStore()
	ctx := context.Background()
	// Create a session object NOT registered in the store (different id) to trigger error.
	sess := core.NewSession("appX", "userX", "missing")
	results := make(chan core.RunResult, 1)
	w := &sessionWriter{runID: "run1", session: sess, store: store, results: results}
	ev := core.NewUserContentEvent("run1")

	err := w.Write(ctx, ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
	assert.Len(t, sess.Events(), 0, "session should not be mutated on failure")
	assert.Empty(t, results)
}

// TestSessionWriter_ContextCanceled ensures canceled context prevents forwarding.
func TestSessionWriter_ContextCanceled(t *testing.T) {
	store := session.NewInMemoryStore()
	ctx := context.Background()
	sess, err := store.GetOrCreate(ctx, "app", "user", "sess1")
	require.NoError(t, err)
	results := make(chan core.RunResult, 1)
	w := &sessionWriter{runID: "run1", session: sess, store: store, results: results}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately
	ev := core.NewUserContentEvent("run1", &core.TextPart{Text: "canceled"})
	err = w.Write(cancelCtx, ev)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// Event still appended (AppendEvent happened before channel send select).
	assert.Len(t, sess.Events(), 1)
	// But no result forwarded because select hit ctx.Done().
	select {
	case <-results:
		assert.Fail(t, "should not have received result on canceled context")
	case <-time.After(50 * time.Millisecond):
		// ok
	}
}
