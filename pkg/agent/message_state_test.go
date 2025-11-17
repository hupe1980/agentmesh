package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// getTextContent extracts text from a message's first part.
// Helper function for testing.
func getTextContent(msg message.Message) string {
	if msg == nil {
		return ""
	}
	parts := msg.Parts()
	if len(parts) == 0 {
		return ""
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		return textPart.Text
	}
	return ""
}

func TestNewMessageState(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)

		if ms == nil {
			t.Fatal("expected MessageState, got nil")
		}
		if ms.Manager() != mgr {
			t.Error("expected underlying manager to match")
		}
		if ms.RunID() != "" {
			t.Errorf("expected empty RunID, got %q", ms.RunID())
		}
		if ms.GraphID() != "" {
			t.Errorf("expected empty GraphID, got %q", ms.GraphID())
		}
	})

	t.Run("with max messages option", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr, WithMaxMessages(10))

		if ms == nil {
			t.Fatal("expected MessageState, got nil")
		}
		// MaxSize is stored in the key but not directly accessible
		// We'll test the behavior in a separate test
	})

	t.Run("with run ID option", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr, WithRunID("test-run-123"))

		if ms.RunID() != "test-run-123" {
			t.Errorf("expected RunID test-run-123, got %q", ms.RunID())
		}
	})

	t.Run("with graph ID option", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr, WithGraphID("graph-456"))

		if ms.GraphID() != "graph-456" {
			t.Errorf("expected GraphID graph-456, got %q", ms.GraphID())
		}
	})

	t.Run("with multiple options", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr,
			WithMaxMessages(50),
			WithRunID("run-789"),
			WithGraphID("graph-789"),
		)

		if ms.RunID() != "run-789" {
			t.Errorf("expected RunID run-789, got %q", ms.RunID())
		}
		if ms.GraphID() != "graph-789" {
			t.Errorf("expected GraphID graph-789, got %q", ms.GraphID())
		}
	})
}

func TestAppendMessages(t *testing.T) {
	t.Run("append single message", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		msg := message.NewHumanMessageFromText("Hello")
		err := ms.AppendMessages(ctx, []message.Message{msg})
		if err != nil {
			t.Fatalf("failed to append message: %v", err)
		}

		// Read back messages
		view, err := mgr.CreateReadView(ctx)
		if err != nil {
			t.Fatalf("failed to create read view: %v", err)
		}

		messages := ms.GetMessages(view)
		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
		if getTextContent(messages[0]) != "Hello" {
			t.Errorf("expected content 'Hello', got %q", getTextContent(messages[0]))
		}
	})

	t.Run("append multiple messages", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
			message.NewAIMessageFromText("Hi there!"),
			message.NewHumanMessageFromText("How are you?"),
		}

		err := ms.AppendMessages(ctx, messages)
		if err != nil {
			t.Fatalf("failed to append messages: %v", err)
		}

		view, _ := mgr.CreateReadView(ctx)
		retrieved := ms.GetMessages(view)

		if len(retrieved) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(retrieved))
		}

		expected := []string{"Hello", "Hi there!", "How are you?"}
		for i, msg := range retrieved {
			if getTextContent(msg) != expected[i] {
				t.Errorf("message %d: expected %q, got %q", i, expected[i], getTextContent(msg))
			}
		}
	})

	t.Run("append messages incrementally", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// First append
		err := ms.AppendMessages(ctx, []message.Message{
			message.NewHumanMessageFromText("Message 1"),
		})
		if err != nil {
			t.Fatalf("first append failed: %v", err)
		}

		// Second append
		err = ms.AppendMessages(ctx, []message.Message{
			message.NewAIMessageFromText("Message 2"),
		})
		if err != nil {
			t.Fatalf("second append failed: %v", err)
		}

		// Third append
		err = ms.AppendMessages(ctx, []message.Message{
			message.NewHumanMessageFromText("Message 3"),
		})
		if err != nil {
			t.Fatalf("third append failed: %v", err)
		}

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		if len(messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(messages))
		}

		expected := []string{"Message 1", "Message 2", "Message 3"}
		for i, msg := range messages {
			if getTextContent(msg) != expected[i] {
				t.Errorf("message %d: expected %q, got %q", i, expected[i], getTextContent(msg))
			}
		}
	})

	t.Run("append empty slice does nothing", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		err := ms.AppendMessages(ctx, []message.Message{})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		if len(messages) != 0 {
			t.Errorf("expected 0 messages, got %d", len(messages))
		}
	})
}

func TestAppendMessage(t *testing.T) {
	t.Run("append single message using convenience method", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		msg := message.NewHumanMessageFromText("Hello")
		err := ms.AppendMessage(ctx, msg)
		if err != nil {
			t.Fatalf("failed to append message: %v", err)
		}

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
	})
}

func TestGetMessages(t *testing.T) {
	t.Run("get messages from empty state", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		if messages == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(messages) != 0 {
			t.Errorf("expected 0 messages, got %d", len(messages))
		}
	})

	t.Run("get messages after appending", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// Append some messages
		ms.AppendMessages(ctx, []message.Message{
			message.NewHumanMessageFromText("First"),
			message.NewAIMessageFromText("Second"),
		})

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}
	})
}

func TestLastMessage(t *testing.T) {
	t.Run("last message from empty state", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		view, _ := mgr.CreateReadView(ctx)
		lastMsg := ms.LastMessage(view)

		if lastMsg != nil {
			t.Errorf("expected nil, got %v", lastMsg)
		}
	})

	t.Run("last message after appending", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// Append messages
		ms.AppendMessages(ctx, []message.Message{
			message.NewHumanMessageFromText("First"),
			message.NewAIMessageFromText("Second"),
			message.NewHumanMessageFromText("Third"),
		})

		view, _ := mgr.CreateReadView(ctx)
		lastMsg := ms.LastMessage(view)

		if lastMsg == nil {
			t.Fatal("expected message, got nil")
		}
		if getTextContent(lastMsg) != "Third" {
			t.Errorf("expected 'Third', got %q", getTextContent(lastMsg))
		}
	})

	t.Run("last message updates correctly", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// First message
		ms.AppendMessage(ctx, message.NewHumanMessageFromText("Message 1"))
		view1, _ := mgr.CreateReadView(ctx)
		last1 := ms.LastMessage(view1)
		if getTextContent(last1) != "Message 1" {
			t.Errorf("expected 'Message 1', got %q", getTextContent(last1))
		}

		// Add second message
		ms.AppendMessage(ctx, message.NewAIMessageFromText("Message 2"))
		view2, _ := mgr.CreateReadView(ctx)
		last2 := ms.LastMessage(view2)
		if getTextContent(last2) != "Message 2" {
			t.Errorf("expected 'Message 2', got %q", getTextContent(last2))
		}
	})
}

func TestMessageCount(t *testing.T) {
	t.Run("count messages", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		view1, _ := mgr.CreateReadView(ctx)
		if ms.MessageCount(view1) != 0 {
			t.Errorf("expected count 0, got %d", ms.MessageCount(view1))
		}

		ms.AppendMessage(ctx, message.NewHumanMessageFromText("First"))
		view2, _ := mgr.CreateReadView(ctx)
		if ms.MessageCount(view2) != 1 {
			t.Errorf("expected count 1, got %d", ms.MessageCount(view2))
		}

		ms.AppendMessages(ctx, []message.Message{
			message.NewAIMessageFromText("Second"),
			message.NewHumanMessageFromText("Third"),
		})
		view3, _ := mgr.CreateReadView(ctx)
		if ms.MessageCount(view3) != 3 {
			t.Errorf("expected count 3, got %d", ms.MessageCount(view3))
		}
	})
}

func TestClearMessages(t *testing.T) {
	t.Run("clear messages", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// Add some messages
		ms.AppendMessages(ctx, []message.Message{
			message.NewHumanMessageFromText("Message 1"),
			message.NewAIMessageFromText("Message 2"),
			message.NewHumanMessageFromText("Message 3"),
		})

		view1, _ := mgr.CreateReadView(ctx)
		if ms.MessageCount(view1) != 3 {
			t.Errorf("expected 3 messages before clear, got %d", ms.MessageCount(view1))
		}

		// Clear messages
		err := ms.ClearMessages(ctx)
		if err != nil {
			t.Fatalf("failed to clear messages: %v", err)
		}

		view2, _ := mgr.CreateReadView(ctx)
		if ms.MessageCount(view2) != 0 {
			t.Errorf("expected 0 messages after clear, got %d", ms.MessageCount(view2))
		}
	})

	t.Run("can append after clearing", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// Add, clear, then add again
		ms.AppendMessage(ctx, message.NewHumanMessageFromText("First"))
		ms.ClearMessages(ctx)
		ms.AppendMessage(ctx, message.NewAIMessageFromText("After clear"))

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
		if getTextContent(messages[0]) != "After clear" {
			t.Errorf("expected 'After clear', got %q", getTextContent(messages[0]))
		}
	})
}

func TestMessageStateWithMaxMessages(t *testing.T) {
	t.Run("respects max messages limit", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr, WithMaxMessages(3))
		ctx := context.Background()

		// Append more messages than the limit
		for i := 1; i <= 5; i++ {
			msg := message.NewHumanMessageFromText(string(rune('A' + i - 1)))
			ms.AppendMessage(ctx, msg)
		}

		view, _ := mgr.CreateReadView(ctx)
		messages := ms.GetMessages(view)

		// Should only keep the last 3 messages
		if len(messages) != 3 {
			t.Fatalf("expected 3 messages (max limit), got %d", len(messages))
		}

		// Should have C, D, E (last 3)
		expected := []string{"C", "D", "E"}
		for i, msg := range messages {
			if getTextContent(msg) != expected[i] {
				t.Errorf("message %d: expected %q, got %q", i, expected[i], getTextContent(msg))
			}
		}
	})
}

func TestMultipleMessageStates(t *testing.T) {
	t.Run("multiple message states can share same manager", func(t *testing.T) {
		mgr := state.NewManager()
		ms1 := NewMessageState(mgr)
		ms2 := NewMessageState(mgr) // Same manager, same messages key
		ctx := context.Background()

		// Append via first MessageState
		ms1.AppendMessage(ctx, message.NewHumanMessageFromText("Hello"))

		// Read via second MessageState
		view, _ := mgr.CreateReadView(ctx)
		messages := ms2.GetMessages(view)

		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
		if getTextContent(messages[0]) != "Hello" {
			t.Errorf("expected 'Hello', got %q", getTextContent(messages[0]))
		}
	})
}

func TestMessageStateImmutability(t *testing.T) {
	t.Run("read view is immutable", func(t *testing.T) {
		mgr := state.NewManager()
		ms := NewMessageState(mgr)
		ctx := context.Background()

		// Append initial messages
		ms.AppendMessages(ctx, []message.Message{
			message.NewHumanMessageFromText("Message 1"),
			message.NewAIMessageFromText("Message 2"),
		})

		// Create view
		view1, _ := mgr.CreateReadView(ctx)
		messages1 := ms.GetMessages(view1)
		count1 := len(messages1)

		// Append more messages
		ms.AppendMessage(ctx, message.NewHumanMessageFromText("Message 3"))

		// Original view should still show old data
		messages1Again := ms.GetMessages(view1)
		if len(messages1Again) != count1 {
			t.Errorf("view mutated: expected %d messages, got %d", count1, len(messages1Again))
		}

		// New view should show updated data
		view2, _ := mgr.CreateReadView(ctx)
		messages2 := ms.GetMessages(view2)
		if len(messages2) != 3 {
			t.Errorf("expected 3 messages in new view, got %d", len(messages2))
		}
	})
}
