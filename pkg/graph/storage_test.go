package graph

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/require"
)

func TestInMemoryStateStore(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStateStore()

	// Create test state
	state := NewStateManager(0).(*State) // Type assert for store.Save
	state.Set("count", 42)
	state.Set("name", "test")
	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessageFromText("hello"),
	}))

	// Test Save
	err := store.Save("checkpoint1", state)
	require.NoError(t, err)

	// Test Load
	loaded, err := store.Load("checkpoint1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, 42, loaded.Get("count"))
	require.Equal(t, "test", loaded.Get("name"))
	require.Len(t, loaded.EventsSnapshot(), 1)

	// Test List
	ids, err := store.List()
	require.NoError(t, err)
	require.Contains(t, ids, "checkpoint1")

	// Test Delete
	err = store.Delete("checkpoint1")
	require.NoError(t, err)

	// Verify deleted
	_, err = store.Load("checkpoint1")
	require.ErrorIs(t, err, ErrCheckpointNotFound)
}

func TestInMemoryStateStore_LoadNonExistent(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStateStore()

	_, err := store.Load("nonexistent")
	require.ErrorIs(t, err, ErrCheckpointNotFound)
}

func TestInMemoryStateStore_SaveNil(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStateStore()

	err := store.Save("test", nil)
	require.ErrorIs(t, err, ErrInvalidState)
}
