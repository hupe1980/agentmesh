package graph

import (
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
)

func TestNewExecutionResult(t *testing.T) {
	msg := message.NewSystemMessageFromText("test message")
	evt := stateif.NewExecutionResult(msg, "graph-123", "test-node")

	assert.NotEmpty(t, evt.ID, "Event ID should be generated")
	assert.Equal(t, "graph-123", evt.GraphID)
	assert.Equal(t, "test-node", evt.Node)
	assert.NotZero(t, evt.Timestamp)
	assert.Equal(t, msg, evt.Message)
}

func TestEvent_UUIDUniqueness(t *testing.T) {
	msg := message.NewSystemMessageFromText("test")
	evt1 := stateif.NewExecutionResult(msg, "graph-1", "node-1")
	evt2 := stateif.NewExecutionResult(msg, "graph-1", "node-1")

	assert.NotEqual(t, evt1.ID, evt2.ID, "Each event should have a unique UUID")
}

func TestEvent_Type(t *testing.T) {
	msg := message.NewAIMessageFromText("AI response")
	evt := stateif.NewExecutionResult(msg, "graph-1", "node-1")

	assert.Equal(t, message.TypeAI, evt.Message.Type())
}

func TestEvent_Parts(t *testing.T) {
	msg := message.NewHumanMessageFromText("Hello")
	evt := stateif.NewExecutionResult(msg, "graph-1", "node-1")

	parts := evt.Message.Parts()
	assert.Len(t, parts, 1)

	textPart, ok := parts[0].(message.TextPart)
	assert.True(t, ok)
	assert.Equal(t, "Hello", textPart.Text)
}

func TestEvent_Clone(t *testing.T) {
	originalMsg := message.NewSystemMessageFromText("original")
	evt := stateif.NewExecutionResult(originalMsg, "graph-123", "node-abc")

	// Allow a small time for timestamp
	time.Sleep(1 * time.Millisecond)

	clonedEvt := evt.Clone()

	assert.IsType(t, &stateif.ExecutionResult{}, clonedEvt)

	// Should preserve all metadata
	assert.Equal(t, evt.ID, clonedEvt.ID)
	assert.Equal(t, evt.GraphID, clonedEvt.GraphID)
	assert.Equal(t, evt.Node, clonedEvt.Node)
	assert.Equal(t, evt.Timestamp, clonedEvt.Timestamp)

	// Should deep copy the message
	assert.Equal(t, evt.Message.Type(), clonedEvt.Message.Type())
	assert.NotSame(t, evt.Message, clonedEvt.Message)
}

func TestEvent_String(t *testing.T) {
	msg := message.NewAIMessageFromText("test")
	evt := stateif.NewExecutionResult(msg, "run-456", "analyzer")

	str := evt.String()
	assert.Contains(t, str, "run-456")
	assert.Contains(t, str, "analyzer")
	assert.Contains(t, str, "ai")
}

func TestEvent_WrapsMessage(t *testing.T) {
	msg := message.NewSystemMessageFromText("test")
	evt := stateif.NewExecutionResult(msg, "graph-1", "node-1")

	// Verify it wraps a message.Message
	assert.NotNil(t, evt.Message)
	assert.Equal(t, message.TypeSystem, evt.Message.Type())
}
