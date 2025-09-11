package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemorySessionStore_GetOrCreate_NewAndExisting(t *testing.T) {
	store := NewInMemoryStore()

	// create new
	s1, err := store.GetOrCreate(context.Background(), "appA", "user1", "sess1")
	require.NoError(t, err)
	require.NotNil(t, s1)
	assert.Equal(t, "appA", s1.AppName())
	assert.Equal(t, "user1", s1.UserID())
	assert.Equal(t, "sess1", s1.ID())

	// mutate clone state locally; should not affect stored session
	s1.SetState("k", "v")

	s2, err := store.GetOrCreate(context.Background(), "appA", "user1", "sess1")
	require.NoError(t, err)
	require.NotNil(t, s2)
	// state in stored session should not include local-only mutation
	_, ok := s2.GetState("k")
	assert.False(t, ok)
}

func TestInMemorySessionStore_AppendEvent_Success(t *testing.T) {
	store := NewInMemoryStore()
	sess, err := store.GetOrCreate(context.Background(), "appA", "user1", "sess1")
	require.NoError(t, err)

	// build an event with state delta and content
	ev := &core.Event{
		Timestamp: time.Now(),
		Parts:     []core.Part{core.NewPartFromText("hello")},
	}
	ev.Actions.StateDelta = core.Map(map[string]any{"a": 1})

	// append should update both the provided session and the stored one
	err = store.AppendEvent(context.Background(), sess, ev)
	require.NoError(t, err)

	// Provided session reflects state and event
	v, ok := sess.GetState("a")
	require.True(t, ok)
	assert.Equal(t, 1, v)
	assert.Len(t, sess.Events(), 1)

	// A fresh clone from store should also contain the event/state
	sess2, err := store.GetOrCreate(context.Background(), "appA", "user1", "sess1")
	require.NoError(t, err)
	v2, ok := sess2.GetState("a")
	require.True(t, ok)
	assert.Equal(t, 1, v2)
	assert.Len(t, sess2.Events(), 1)
}

func TestInMemorySessionStore_AppendEvent_NotFoundPaths(t *testing.T) {
	store := NewInMemoryStore()

	// Missing app
	sessMissingApp := core.NewSession("no-app", "u", "s")
	ev := &core.Event{Timestamp: time.Now()}
	err := store.AppendEvent(context.Background(), sessMissingApp, ev)
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrSessionNotFound))
	// Ensure the provided session was not mutated on failure
	assert.Len(t, sessMissingApp.Events(), 0)

	// Ensure app exists in store
	_, err = store.GetOrCreate(context.Background(), "appA", "user1", "sess1")
	require.NoError(t, err)

	// Missing user
	sessMissingUser := core.NewSession("appA", "no-user", "s")
	err = store.AppendEvent(context.Background(), sessMissingUser, &core.Event{Timestamp: time.Now()})
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrSessionNotFound))
	assert.Len(t, sessMissingUser.Events(), 0)

	// Ensure user exists in store
	_, err = store.GetOrCreate(context.Background(), "appA", "user2", "sessX")
	require.NoError(t, err)

	// Missing session id under existing app/user
	sessMissingID := core.NewSession("appA", "user2", "no-sess")
	err = store.AppendEvent(context.Background(), sessMissingID, &core.Event{Timestamp: time.Now()})
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrSessionNotFound))
	assert.Len(t, sessMissingID.Events(), 0)
}

func TestInMemorySessionStore_Close_NoOp(t *testing.T) {
	store := NewInMemoryStore()
	assert.NoError(t, store.Close())
}
