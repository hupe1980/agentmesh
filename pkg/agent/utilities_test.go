package agent

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMessages(t *testing.T) {
	t.Run("returns empty slice when no messages", func(t *testing.T) {
		scope := testutil.NewTestScopeFromMap[message.Message](nil)
		msgs := GetMessages(scope)
		assert.Empty(t, msgs)
	})

	t.Run("returns messages from scope", func(t *testing.T) {
		input := []message.Message{
			message.NewHumanMessageFromText("Hello"),
			message.NewAIMessageFromText("World"),
		}
		scope := testutil.NewTestScopeFromMap[message.Message](map[string]any{
			MessagesKey.Name(): input,
		})

		msgs := GetMessages(scope)
		require.Len(t, msgs, 2)
		assert.Equal(t, "Hello", msgs[0].String())
		assert.Equal(t, "World", msgs[1].String())
	})
}

func TestLastMessage(t *testing.T) {
	t.Run("returns nil when no messages", func(t *testing.T) {
		scope := testutil.NewTestScopeFromMap[message.Message](nil)
		last := LastMessage(scope)
		assert.Nil(t, last)
	})

	t.Run("returns last message", func(t *testing.T) {
		input := []message.Message{
			message.NewHumanMessageFromText("First"),
			message.NewAIMessageFromText("Last message"),
		}
		scope := testutil.NewTestScopeFromMap[message.Message](map[string]any{
			MessagesKey.Name(): input,
		})

		last := LastMessage(scope)
		require.NotNil(t, last)
		assert.Equal(t, "Last message", last.String())
	})
}

func TestIsConversationalContext(t *testing.T) {
	t.Run("returns false for single human message", func(t *testing.T) {
		input := []message.Message{
			message.NewHumanMessageFromText("Single question"),
		}
		scope := testutil.NewTestScopeFromMap[message.Message](map[string]any{
			MessagesKey.Name(): input,
		})

		isConv := IsConversationalContext(scope)
		assert.False(t, isConv)
	})

	t.Run("returns true when AI response exists", func(t *testing.T) {
		input := []message.Message{
			message.NewHumanMessageFromText("Question"),
			message.NewAIMessageFromText("Answer"),
			message.NewHumanMessageFromText("Follow-up"),
		}
		scope := testutil.NewTestScopeFromMap[message.Message](map[string]any{
			MessagesKey.Name(): input,
		})

		isConv := IsConversationalContext(scope)
		assert.True(t, isConv)
	})

	t.Run("returns true for multiple human messages", func(t *testing.T) {
		input := []message.Message{
			message.NewHumanMessageFromText("First question"),
			message.NewHumanMessageFromText("Second question"),
		}
		scope := testutil.NewTestScopeFromMap[message.Message](map[string]any{
			MessagesKey.Name(): input,
		})

		isConv := IsConversationalContext(scope)
		assert.True(t, isConv)
	})

	t.Run("returns true when memory context exists", func(t *testing.T) {
		input := []message.Message{
			message.NewHumanMessageFromText("Single question"),
		}
		memoryContext := []message.Message{
			message.NewHumanMessageFromText("Previous question"),
			message.NewAIMessageFromText("Previous answer"),
		}
		scope := testutil.NewTestScopeFromMap[message.Message](map[string]any{
			MessagesKey.Name():       input,
			MemoryContextKey.Name(): memoryContext,
		})

		isConv := IsConversationalContext(scope)
		assert.True(t, isConv)
	})

	t.Run("returns false for empty messages", func(t *testing.T) {
		scope := testutil.NewTestScopeFromMap[message.Message](nil)

		isConv := IsConversationalContext(scope)
		assert.False(t, isConv)
	})
}

func TestGetConversationHistory(t *testing.T) {
	t.Run("returns nil for empty messages", func(t *testing.T) {
		history := GetConversationHistory(nil)
		assert.Nil(t, history)
	})

	t.Run("returns nil for single message", func(t *testing.T) {
		msgs := []message.Message{
			message.NewHumanMessageFromText("Single message"),
		}
		history := GetConversationHistory(msgs)
		assert.Nil(t, history)
	})

	t.Run("returns history excluding last human message", func(t *testing.T) {
		msgs := []message.Message{
			message.NewHumanMessageFromText("First question"),
			message.NewAIMessageFromText("First answer"),
			message.NewHumanMessageFromText("Follow-up"),
		}
		history := GetConversationHistory(msgs)
		require.Len(t, history, 2)
		assert.Equal(t, "First question", history[0].String())
		assert.Equal(t, "First answer", history[1].String())
	})

	t.Run("returns nil when last human is first message", func(t *testing.T) {
		msgs := []message.Message{
			message.NewHumanMessageFromText("Only human message"),
		}
		history := GetConversationHistory(msgs)
		assert.Nil(t, history)
	})

	t.Run("handles multiple exchanges", func(t *testing.T) {
		msgs := []message.Message{
			message.NewHumanMessageFromText("Q1"),
			message.NewAIMessageFromText("A1"),
			message.NewHumanMessageFromText("Q2"),
			message.NewAIMessageFromText("A2"),
			message.NewHumanMessageFromText("Q3"),
		}
		history := GetConversationHistory(msgs)
		require.Len(t, history, 4)
		assert.Equal(t, "Q1", history[0].String())
		assert.Equal(t, "A1", history[1].String())
		assert.Equal(t, "Q2", history[2].String())
		assert.Equal(t, "A2", history[3].String())
	})

	t.Run("handles AI-only trailing messages", func(t *testing.T) {
		msgs := []message.Message{
			message.NewHumanMessageFromText("Question"),
			message.NewAIMessageFromText("Answer"),
		}
		// Last human is at index 0, so history should be nil
		history := GetConversationHistory(msgs)
		assert.Nil(t, history)
	})
}
