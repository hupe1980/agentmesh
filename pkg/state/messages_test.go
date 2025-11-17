package state

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessagesKey(t *testing.T) {
	t.Run("messages key has correct name", func(t *testing.T) {
		assert.Equal(t, "__messages__", MessagesKey.Name())
	})

	t.Run("messages key is unbounded", func(t *testing.T) {
		assert.Equal(t, 0, MessagesKey.MaxSize())
	})
}

func TestGetMessages(t *testing.T) {
	t.Run("get messages from empty state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		snap := s.Snapshot()
		view := NewReadView(snap)

		msgs := GetMessages(view)
		assert.Empty(t, msgs)
	})

	t.Run("get messages from state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		// Add messages
		ctx := context.Background()
		results := []ExecutionResult{
			{
				Message:   message.NewHumanMessageFromText("Hello"),
				ID:        "1",
				Node:      "start",
				Timestamp: time.Now(),
			},
			{
				Message:   message.NewAIMessageFromText("Hi there!"),
				ID:        "2",
				Node:      "model",
				Timestamp: time.Now(),
			},
		}

		updates := Updates{}
		AppendMessages(updates, results)
		err := s.ApplyUpdates(ctx, updates)
		require.NoError(t, err)

		snap := s.Snapshot()
		view := NewReadView(snap)

		msgs := GetMessages(view)
		assert.Len(t, msgs, 2)
		assert.Equal(t, "1", msgs[0].ID)
		assert.Equal(t, "2", msgs[1].ID)
	})
}

func TestAppendMessages(t *testing.T) {
	t.Run("append empty messages does nothing", func(t *testing.T) {
		updates := Updates{}
		AppendMessages(updates, []ExecutionResult{})

		assert.Empty(t, updates)
	})

	t.Run("append messages to updates", func(t *testing.T) {
		results := []ExecutionResult{
			{
				Message:   message.NewHumanMessageFromText("test"),
				ID:        "1",
				Node:      "start",
				Timestamp: time.Now(),
			},
		}

		updates := Updates{}
		AppendMessages(updates, results)

		assert.Contains(t, updates, "__messages__")
		assert.Equal(t, results, updates["__messages__"])
	})

	t.Run("append messages accumulates in state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		ctx := context.Background()

		// First batch
		updates1 := Updates{}
		AppendMessages(updates1, []ExecutionResult{
			{Message: message.NewHumanMessageFromText("msg1"), ID: "1"},
		})
		err := s.ApplyUpdates(ctx, updates1)
		require.NoError(t, err)

		// Second batch
		updates2 := Updates{}
		AppendMessages(updates2, []ExecutionResult{
			{Message: message.NewHumanMessageFromText("msg2"), ID: "2"},
		})
		err = s.ApplyUpdates(ctx, updates2)
		require.NoError(t, err)

		snap := s.Snapshot()
		view := NewReadView(snap)
		msgs := GetMessages(view)

		assert.Len(t, msgs, 2)
		assert.Equal(t, "1", msgs[0].ID)
		assert.Equal(t, "2", msgs[1].ID)
	})
}

func TestExtractMessageContent(t *testing.T) {
	t.Run("extract from empty results", func(t *testing.T) {
		content := ExtractMessageContent([]ExecutionResult{})
		assert.Nil(t, content)
	})

	t.Run("extract from nil results", func(t *testing.T) {
		content := ExtractMessageContent(nil)
		assert.Nil(t, content)
	})

	t.Run("extract message content", func(t *testing.T) {
		humanMsg := message.NewHumanMessageFromText("Hello")
		aiMsg := message.NewAIMessageFromText("Hi")

		results := []ExecutionResult{
			{
				Message:   humanMsg,
				ID:        "1",
				Node:      "start",
				Timestamp: time.Now(),
			},
			{
				Message:   aiMsg,
				ID:        "2",
				Node:      "model",
				Timestamp: time.Now(),
			},
		}

		content := ExtractMessageContent(results)

		require.Len(t, content, 2)
		assert.Equal(t, humanMsg, content[0])
		assert.Equal(t, aiMsg, content[1])
	})

	t.Run("extracted content has no metadata", func(t *testing.T) {
		results := []ExecutionResult{
			{
				Message:   message.NewHumanMessageFromText("test"),
				ID:        "123",
				Node:      "node-name",
				GraphID:   "graph-123",
				Timestamp: time.Now(),
			},
		}

		content := ExtractMessageContent(results)

		// Content should just be the message, not ExecutionResult
		require.Len(t, content, 1)
		assert.Equal(t, message.TypeHuman, content[0].Type())
	})
}

func TestLastMessage(t *testing.T) {
	t.Run("last message from empty state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		snap := s.Snapshot()
		view := NewReadView(snap)

		last := LastMessage(view)
		assert.Nil(t, last)
	})

	t.Run("last message from state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		ctx := context.Background()
		results := []ExecutionResult{
			{Message: message.NewHumanMessageFromText("first"), ID: "1"},
			{Message: message.NewHumanMessageFromText("second"), ID: "2"},
			{Message: message.NewHumanMessageFromText("third"), ID: "3"},
		}

		updates := Updates{}
		AppendMessages(updates, results)
		s.ApplyUpdates(ctx, updates)

		snap := s.Snapshot()
		view := NewReadView(snap)

		last := LastMessage(view)
		require.NotNil(t, last)
		assert.Equal(t, "3", last.ID)
	})
}

func TestLastMessageContent(t *testing.T) {
	t.Run("last message content from empty state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		snap := s.Snapshot()
		view := NewReadView(snap)

		content := LastMessageContent(view)
		assert.Nil(t, content)
	})

	t.Run("last message content from state", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)

		ctx := context.Background()
		lastMsg := message.NewAIMessageFromText("final response")

		results := []ExecutionResult{
			{Message: message.NewHumanMessageFromText("first"), ID: "1"},
			{Message: lastMsg, ID: "2"},
		}

		updates := Updates{}
		AppendMessages(updates, results)
		s.ApplyUpdates(ctx, updates)

		snap := s.Snapshot()
		view := NewReadView(snap)

		content := LastMessageContent(view)
		require.NotNil(t, content)
		assert.Equal(t, message.TypeAI, content.Type())
		assert.Equal(t, lastMsg, content)
	})
}

func TestMessageWorkflow(t *testing.T) {
	t.Run("complete message workflow", func(t *testing.T) {
		// Simulate a conversation workflow
		s := NewState()
		RegisterList(s, MessagesKey)
		ctx := context.Background()

		// Step 1: User sends message
		updates1 := Updates{}
		AppendMessages(updates1, []ExecutionResult{
			{
				Message:   message.NewHumanMessageFromText("What is 2+2?"),
				ID:        "user-1",
				Node:      "input",
				Timestamp: time.Now(),
			},
		})
		s.ApplyUpdates(ctx, updates1)

		// Step 2: AI responds
		updates2 := Updates{}
		AppendMessages(updates2, []ExecutionResult{
			{
				Message:   message.NewAIMessageFromText("2+2 equals 4"),
				ID:        "ai-1",
				Node:      "model",
				Timestamp: time.Now(),
			},
		})
		s.ApplyUpdates(ctx, updates2)

		// Step 3: User follows up
		updates3 := Updates{}
		AppendMessages(updates3, []ExecutionResult{
			{
				Message:   message.NewHumanMessageFromText("Thanks!"),
				ID:        "user-2",
				Node:      "input",
				Timestamp: time.Now(),
			},
		})
		s.ApplyUpdates(ctx, updates3)

		// Verify complete conversation
		snap := s.Snapshot()
		view := NewReadView(snap)

		msgs := GetMessages(view)
		assert.Len(t, msgs, 3)

		content := ExtractMessageContent(msgs)
		assert.Equal(t, message.TypeHuman, content[0].Type())
		assert.Equal(t, message.TypeAI, content[1].Type())
		assert.Equal(t, message.TypeHuman, content[2].Type())

		last := LastMessageContent(view)
		assert.Equal(t, message.TypeHuman, last.Type())
	})
}

func TestMessageTypes(t *testing.T) {
	t.Run("different message types", func(t *testing.T) {
		s := NewState()
		RegisterList(s, MessagesKey)
		ctx := context.Background()

		results := []ExecutionResult{
			{Message: message.NewSystemMessageFromText("system prompt"), ID: "1"},
			{Message: message.NewHumanMessageFromText("user input"), ID: "2"},
			{Message: message.NewAIMessageFromText("ai response"), ID: "3"},
			{Message: message.NewToolMessage("tool-1", "tool result"), ID: "4"},
		}

		updates := Updates{}
		AppendMessages(updates, results)
		s.ApplyUpdates(ctx, updates)

		snap := s.Snapshot()
		view := NewReadView(snap)
		content := ExtractMessageContent(GetMessages(view))

		assert.Len(t, content, 4)
		assert.Equal(t, message.TypeSystem, content[0].Type())
		assert.Equal(t, message.TypeHuman, content[1].Type())
		assert.Equal(t, message.TypeAI, content[2].Type())
		assert.Equal(t, message.TypeTool, content[3].Type())
	})
}
