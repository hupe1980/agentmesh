package graph

import (
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
)

func TestNewMessageEvent(t *testing.T) {
	msg := message.NewSystemMessageFromText("test message")
	evt := NewMessageEvent(msg, "graph-123", "test-node")

	assert.NotEmpty(t, evt.ID, "Event ID should be generated")
	assert.Equal(t, "graph-123", evt.GraphID)
	assert.Equal(t, "test-node", evt.Node)
	assert.NotZero(t, evt.Timestamp)
	assert.Equal(t, msg, evt.Message)
}

func TestMessageEvent_UUIDUniqueness(t *testing.T) {
	msg := message.NewSystemMessageFromText("test")
	evt1 := NewMessageEvent(msg, "graph-1", "node-1")
	evt2 := NewMessageEvent(msg, "graph-1", "node-1")

	assert.NotEqual(t, evt1.ID, evt2.ID, "Each event should have a unique UUID")
}

func TestMessageEvent_Type(t *testing.T) {
	msg := message.NewAIMessageFromText("AI response")
	evt := NewMessageEvent(msg, "graph-1", "node-1")

	assert.Equal(t, message.TypeAI, evt.Type())
}

func TestMessageEvent_Parts(t *testing.T) {
	msg := message.NewHumanMessageFromText("Hello")
	evt := NewMessageEvent(msg, "graph-1", "node-1")

	parts := evt.Parts()
	assert.Len(t, parts, 1)

	textPart, ok := parts[0].(message.TextPart)
	assert.True(t, ok)
	assert.Equal(t, "Hello", textPart.Text)
}

func TestMessageEvent_Clone(t *testing.T) {
	originalMsg := message.NewSystemMessageFromText("original")
	evt := NewMessageEvent(originalMsg, "graph-123", "node-abc")

	// Allow a small time for timestamp
	time.Sleep(1 * time.Millisecond)

	cloned := evt.Clone()

	assert.IsType(t, &MessageEvent{}, cloned)
	clonedEvt := cloned.(*MessageEvent)

	// Should preserve all metadata
	assert.Equal(t, evt.ID, clonedEvt.ID)
	assert.Equal(t, evt.GraphID, clonedEvt.GraphID)
	assert.Equal(t, evt.Node, clonedEvt.Node)
	assert.Equal(t, evt.Timestamp, clonedEvt.Timestamp)

	// Should deep copy the message
	assert.Equal(t, evt.Message.Type(), clonedEvt.Message.Type())
	assert.NotSame(t, evt.Message, clonedEvt.Message)
}

func TestMessageEvent_String(t *testing.T) {
	msg := message.NewAIMessageFromText("test")
	evt := NewMessageEvent(msg, "run-456", "analyzer")

	str := evt.String()
	assert.Contains(t, str, "run-456")
	assert.Contains(t, str, "analyzer")
	assert.Contains(t, str, "ai")
}

func TestMessageEvent_ImplementsMessageInterface(t *testing.T) {
	msg := message.NewSystemMessageFromText("test")
	evt := NewMessageEvent(msg, "graph-1", "node-1")

	// Verify it implements message.Message interface
	var _ message.Message = evt
	var _ message.Message = (*MessageEvent)(nil)
}
