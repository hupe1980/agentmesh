package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryMemoryStore_AddSessionAndSearch(t *testing.T) {
	svc := NewInMemoryStore()

	// Create sessions with events
	for i := range 5 {
		session := core.NewSession("app", "user", fmt.Sprintf("s2-%d", i))
		event := &core.Event{Parts: []core.Part{core.NewPartFromText("contentA")}}
		session.AddEvent(event)
		err := svc.AddSession(context.Background(), session)
		require.NoError(t, err)
	}

	// Search all (empty query)
	res, err := svc.Search(context.Background(), "app", "user", "")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Len(t, res.Memories, 5)

	// Search with query substring
	res2, _ := svc.Search(context.Background(), "app", "user", "contentA")
	// all 5 events match the query
	assert.Len(t, res2.Memories, 5)
	// verify content contains text
	require.NotEmpty(t, res2.Memories)
	if tp, ok := res2.Memories[0].Parts[0].(*core.TextPart); ok {
		assert.Contains(t, tp.Text, "contentA")
	} else {
		t.Fatalf("expected TextPart, got %T", res2.Memories[0].Parts[0])
	}

	// Search with query not present
	res3, _ := svc.Search(context.Background(), "app", "user", "notfound")
	assert.Len(t, res3.Memories, 0)
}

func TestInMemoryMemoryStore_EmptySearch(t *testing.T) {
	svc := NewInMemoryStore()
	res, err := svc.Search(context.Background(), "app", "user", "")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Memories)
}

func TestInMemoryMemoryStore_DefensiveCloning(t *testing.T) {
	svc := NewInMemoryStore()

	// Build session with a text part
	s := core.NewSession("app", "user", "sess1")
	tp := core.NewPartFromText("hello")
	ev := &core.Event{Parts: []core.Part{tp}}
	s.AddEvent(ev)

	// Add session; store should clone
	require.NoError(t, svc.AddSession(context.Background(), s))

	// Mutate original event parts after saving
	if tpp, ok := tp.(*core.TextPart); ok {
		tpp.Text = "MUTATED"
	}

	// Search and verify stored is unchanged and also returns a clone
	res, err := svc.Search(context.Background(), "app", "user", "hello")
	require.NoError(t, err)
	require.Len(t, res.Memories, 1)
	part := res.Memories[0].Parts[0]
	tpp2, ok := part.(*core.TextPart)
	require.True(t, ok)
	assert.Equal(t, "hello", tpp2.Text)

	// Mutate returned part; search again remains original
	tpp2.Text = "XXX"
	res2, err := svc.Search(context.Background(), "app", "user", "hello")
	require.NoError(t, err)
	require.Len(t, res2.Memories, 1)
	tpp3 := res2.Memories[0].Parts[0].(*core.TextPart)
	assert.Equal(t, "hello", tpp3.Text)
}

func TestInMemoryMemoryStore_ErrMemoryNotFoundWhenOrderMissing(t *testing.T) {
	svc := NewInMemoryStore()

	// Simulate legacy/partial state: session map exists but no insertion order recorded
	userKey := userKey("app", "user")
	svc.mu.Lock()
	svc.sessionEvents[userKey] = map[string][]*core.Event{
		"sess1": {
			{ // placeholder, will be replaced below to avoid nil pointer
			},
		},
	}
	// Replace placeholder with a proper event pointer
	svc.sessionEvents[userKey]["sess1"][0] = &core.Event{Parts: []core.Part{core.NewPartFromText("x")}}
	svc.mu.Unlock()

	res, err := svc.Search(context.Background(), "app", "user", "")
	assert.Nil(t, res)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrMemoryNotFound)
}
