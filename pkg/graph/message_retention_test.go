package graph

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to extract text from a message for assertions
func getMessageText(msg message.Message) string {
	parts := msg.Parts()
	if len(parts) == 0 {
		return ""
	}
	if tp, ok := parts[0].(message.TextPart); ok {
		return tp.Text
	}
	return ""
}

func TestState_MessageRetention_Default(t *testing.T) {
	// Default behavior: unlimited retention
	state := NewStateManager(0)

	messages := []message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg1"}}),
		message.NewAIMessageFromText("msg2"),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg3"}}),
	}

	state.AddMessages(wrapMessages(messages))
	retrieved := state.EventsSnapshot()

	assert.Len(t, retrieved, 3, "All messages should be retained by default")
}

func TestState_SetMaxMessages_Zero(t *testing.T) {
	// Zero means unlimited
	state := NewStateManager(0)
	// MaxMessages now set at creation: NewStateManager(0)

	messages := make([]message.Message, 150)
	for i := range messages {
		messages[i] = message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg"}})
	}

	state.AddMessages(wrapMessages(messages))
	retrieved := state.EventsSnapshot()

	assert.Len(t, retrieved, 150, "Zero limit should allow unlimited messages")
}

func TestState_SetMaxMessages_EnforceLimit(t *testing.T) {
	// Set limit at construction
	state := NewStateManager(5)

	messages := make([]message.Message, 10)
	for i := range messages {
		messages[i] = message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg"}})
	}

	state.AddMessages(wrapMessages(messages))
	retrieved := state.EventsSnapshot()

	require.Len(t, retrieved, 5, "Should retain only maxMessages")
}

func TestState_SetMaxMessages_KeepsMostRecent(t *testing.T) {
	// Verify oldest messages are discarded, newest are kept
	state := NewStateManager(3)

	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "old1"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "old2"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "keep1"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "keep2"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "keep3"}}),
	}))

	retrieved := state.EventsSnapshot()
	require.Len(t, retrieved, 3, "Should have exactly 3 messages")

	// Check that we kept the last 3
	assert.Equal(t, "keep1", getMessageText(retrieved[0].Message))
	assert.Equal(t, "keep2", getMessageText(retrieved[1].Message))
	assert.Equal(t, "keep3", getMessageText(retrieved[2].Message))
}

func TestState_SetMaxMessages_MultipleAdds(t *testing.T) {
	// Multiple AddMessages calls should still respect limit
	state := NewStateManager(4)

	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg1"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg2"}}),
	}))

	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg3"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg4"}}),
	}))

	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg5"}}),
	}))

	retrieved := state.EventsSnapshot()
	require.Len(t, retrieved, 4, "Should enforce limit across multiple adds")

	// Should have msg2, msg3, msg4, msg5 (dropped msg1)
	assert.Equal(t, "msg2", getMessageText(retrieved[0].Message))
	assert.Equal(t, "msg3", getMessageText(retrieved[1].Message))
	assert.Equal(t, "msg4", getMessageText(retrieved[2].Message))
	assert.Equal(t, "msg5", getMessageText(retrieved[3].Message))
}

func TestState_SetMaxMessages_ApplyAfterMessages(t *testing.T) {
	// Setting limit at construction should immediately enforce it
	state := NewStateManager(2)

	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg1"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg2"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg3"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg4"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg5"}}),
	}))

	retrieved := state.EventsSnapshot()
	require.Len(t, retrieved, 2, "Should enforce limit immediately")

	assert.Equal(t, "msg4", getMessageText(retrieved[0].Message))
	assert.Equal(t, "msg5", getMessageText(retrieved[1].Message))
}

func TestState_SetMaxMessages_Negative(t *testing.T) {
	// Negative values should be treated as zero (unlimited)
	// Note: maxMessages is now set at construction, so we test with 0
	state := NewStateManager(0) // 0 means unlimited

	messages := make([]message.Message, 20)
	for i := range messages {
		messages[i] = message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg"}})
	}

	state.AddMessages(wrapMessages(messages))
	retrieved := state.EventsSnapshot()

	assert.Len(t, retrieved, 20, "Negative limit should be treated as unlimited")
}

func TestState_ApplyUpdates_RespectsLimit(t *testing.T) {
	// ApplyUpdates should also respect message limits
	state := NewStateManager(3)

	state.AddMessages(wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg1"}}),
	}))

	// ApplyUpdates adds messages
	state.ApplyUpdates(nil, wrapMessages([]message.Message{
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg2"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg3"}}),
		message.NewHumanMessage(message.Parts{message.TextPart{Text: "msg4"}}),
	}))

	retrieved := state.EventsSnapshot()
	require.Len(t, retrieved, 3, "ApplyUpdates should enforce limit")

	assert.Equal(t, "msg2", getMessageText(retrieved[0].Message))
	assert.Equal(t, "msg3", getMessageText(retrieved[1].Message))
	assert.Equal(t, "msg4", getMessageText(retrieved[2].Message))
}

func TestState_MessageRetention_EmptyMessages(t *testing.T) {
	state := NewStateManager(0)
	// MaxMessages now set at creation: NewStateManager(5)

	// Adding empty slice should not panic
	state.AddMessages(wrapMessages([]message.Message{}))
	retrieved := state.EventsSnapshot()

	assert.Len(t, retrieved, 0, "No messages added")
}

func TestWithMaxMessages_Option(t *testing.T) {
	// Test the RunOption function
	options := defaultRunOptions()

	opt := WithMaxMessages(100)
	opt(&options)

	assert.Equal(t, 100, options.maxMessages, "Option should set maxMessages")
}

func TestWithMaxMessages_Zero(t *testing.T) {
	options := defaultRunOptions()

	opt := WithMaxMessages(0)
	opt(&options)

	assert.Equal(t, 0, options.maxMessages, "Zero should be allowed")
}

func TestWithMaxMessages_Negative(t *testing.T) {
	options := defaultRunOptions()
	options.maxMessages = 50 // Set initial value

	opt := WithMaxMessages(-10)
	opt(&options)

	// Negative should be ignored, keeps original
	assert.Equal(t, 50, options.maxMessages, "Negative value should be ignored")
}

func TestWithMaxMessages_NilOptions(t *testing.T) {
	// Option should handle nil gracefully (though this shouldn't happen in practice)
	opt := WithMaxMessages(100)

	// Should not panic
	assert.NotPanics(t, func() {
		opt(nil)
	}, "Should handle nil options without panicking")
}

// Helper function for tests to wrap messages as events

func wrapMessages(msgs []message.Message) []Event {
	events := make([]Event, len(msgs))
	for i, msg := range msgs {
		events[i] = *NewEvent(msg, "", "test")
	}
	return events
}
