package state

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessagesKeyNew(t *testing.T) {
	t.Run("messages key has correct name", func(t *testing.T) {
		assert.Equal(t, "__messages__", MessagesKey.Name())
	})

	t.Run("messages key is unbounded", func(t *testing.T) {
		assert.Equal(t, 0, MessagesKey.MaxSize())
	})
}

func TestGetMessagesNew(t *testing.T) {
	t.Run("get messages from empty manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)

		view, err := mgr.CreateReadView(context.Background())
		require.NoError(t, err)

		msgs := GetMessages(view)
		assert.Empty(t, msgs)
	})

	t.Run("get messages from manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)

		// Add messages
		ctx := context.Background()
		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
			message.NewAIMessageFromText("Hi there!"),
		}

		updates := Updates{}
		AppendMessages(updates, messages)
		err := ApplyUpdates(ctx, mgr, updates)
		require.NoError(t, err)

		view, err := mgr.CreateReadView(ctx)
		require.NoError(t, err)

		msgs := GetMessages(view)
		assert.Len(t, msgs, 2)
		assert.Equal(t, message.TypeHuman, msgs[0].Type())
		assert.Equal(t, message.TypeAI, msgs[1].Type())
	})
}

func TestAppendMessagesNew(t *testing.T) {
	t.Run("append empty messages does nothing", func(t *testing.T) {
		updates := Updates{}
		AppendMessages(updates, []message.Message{})

		assert.Empty(t, updates)
	})

	t.Run("append messages to updates", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText("Test"),
		}

		updates := Updates{}
		AppendMessages(updates, messages)

		assert.Contains(t, updates, MessagesKey.Name())
		assert.Len(t, updates[MessagesKey.Name()].([]message.Message), 1)
	})

	t.Run("append messages accumulates in manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)
		ctx := context.Background()

		// First batch
		updates1 := Updates{}
		AppendMessages(updates1, []message.Message{
			message.NewHumanMessageFromText("msg1"),
		})
		err := ApplyUpdates(ctx, mgr, updates1)
		require.NoError(t, err)

		// Second batch
		updates2 := Updates{}
		AppendMessages(updates2, []message.Message{
			message.NewHumanMessageFromText("msg2"),
		})
		err = ApplyUpdates(ctx, mgr, updates2)
		require.NoError(t, err)

		// Verify both messages exist
		view, err := mgr.CreateReadView(ctx)
		require.NoError(t, err)

		msgs := GetMessages(view)
		assert.Len(t, msgs, 2)
		assert.Equal(t, message.TypeHuman, msgs[0].Type())
		assert.Equal(t, message.TypeHuman, msgs[1].Type())
	})
}

func TestExtractMessageContentNew(t *testing.T) {
	t.Run("extract from empty messages", func(t *testing.T) {
		content := ExtractMessageContent([]message.Message{})
		assert.Empty(t, content) // Returns empty slice, not nil
	})

	t.Run("extract message content - pass through", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
			message.NewAIMessageFromText("Hi"),
		}

		content := ExtractMessageContent(messages)
		assert.Len(t, content, 2)
		assert.Equal(t, messages, content) // Should be pass-through
	})
}

func TestLastMessageNew(t *testing.T) {
	t.Run("last message from empty manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)

		view, err := mgr.CreateReadView(context.Background())
		require.NoError(t, err)

		last := LastMessage(view)
		assert.Nil(t, last)
	})

	t.Run("last message from manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)
		ctx := context.Background()

		messages := []message.Message{
			message.NewHumanMessageFromText("First"),
			message.NewAIMessageFromText("Second"),
			message.NewHumanMessageFromText("Third"),
		}

		updates := Updates{}
		AppendMessages(updates, messages)
		err := ApplyUpdates(ctx, mgr, updates)
		require.NoError(t, err)

		view, err := mgr.CreateReadView(ctx)
		require.NoError(t, err)

		last := LastMessage(view)
		assert.NotNil(t, last)
		assert.Equal(t, message.TypeHuman, last.Type())
	})
}

func TestLastMessageContentNew(t *testing.T) {
	t.Run("last message content from empty manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)

		view, err := mgr.CreateReadView(context.Background())
		require.NoError(t, err)

		last := LastMessageContent(view)
		assert.Nil(t, last)
	})

	t.Run("last message content from manager", func(t *testing.T) {
		mgr := NewManager()
		RegisterListKey(mgr, MessagesKey)
		ctx := context.Background()

		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
			message.NewAIMessageFromText("Hi there!"),
		}

		updates := Updates{}
		AppendMessages(updates, messages)
		err := ApplyUpdates(ctx, mgr, updates)
		require.NoError(t, err)

		view, err := mgr.CreateReadView(ctx)
		require.NoError(t, err)

		last := LastMessageContent(view)
		assert.NotNil(t, last)
		assert.Equal(t, message.TypeAI, last.Type())
	})
}
