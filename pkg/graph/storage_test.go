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
	state := NewGraphState(0)
	state.Set("count", 42)
	state.Set("name", "test")
	state.AddMessages([]message.Message{
		message.NewHumanMessageFromText("hello"),
	})

	// Test Save
	err := store.Save("checkpoint1", state)
	require.NoError(t, err)

	// Test Load
	loaded, err := store.Load("checkpoint1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, 42, loaded.Get("count"))
	require.Equal(t, "test", loaded.Get("name"))
	require.Len(t, loaded.MessagesSnapshot(), 1)

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

func TestInMemoryMessageBus(t *testing.T) {
	t.Parallel()

	bus := NewInMemoryMessageBus()

	msgs := []message.Message{
		message.NewHumanMessageFromText("msg1"),
		message.NewHumanMessageFromText("msg2"),
	}

	// Test Send
	err := bus.Send("vertex1", msgs)
	require.NoError(t, err)

	// Test Receive
	received, err := bus.Receive("vertex1")
	require.NoError(t, err)
	require.Len(t, received, 2)

	// Mailbox should be cleared after receive
	received, err = bus.Receive("vertex1")
	require.NoError(t, err)
	require.Nil(t, received)
}

func TestInMemoryMessageBus_Clear(t *testing.T) {
	t.Parallel()

	bus := NewInMemoryMessageBus()

	msgs := []message.Message{message.NewHumanMessageFromText("test")}
	err := bus.Send("vertex1", msgs)
	require.NoError(t, err)

	// Clear mailbox
	err = bus.Clear("vertex1")
	require.NoError(t, err)

	// Should be empty
	received, err := bus.Receive("vertex1")
	require.NoError(t, err)
	require.Nil(t, received)
}

func TestInMemoryMessageBus_MultipleVertices(t *testing.T) {
	t.Parallel()

	bus := NewInMemoryMessageBus()

	// Send to different vertices
	err := bus.Send("v1", []message.Message{message.NewHumanMessageFromText("to v1")})
	require.NoError(t, err)

	err = bus.Send("v2", []message.Message{message.NewHumanMessageFromText("to v2")})
	require.NoError(t, err)

	// Receive from v1
	msgs1, err := bus.Receive("v1")
	require.NoError(t, err)
	require.Len(t, msgs1, 1)

	// Receive from v2
	msgs2, err := bus.Receive("v2")
	require.NoError(t, err)
	require.Len(t, msgs2, 1)

	// Verify independence
	require.NotEqual(t, msgs1[0], msgs2[0])
}
