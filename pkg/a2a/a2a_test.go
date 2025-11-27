package a2a

import (
	"context"
	"testing"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToA2AMessage_SystemMessage(t *testing.T) {
	msg := message.NewSystemMessage([]message.Part{
		message.TextPart{Text: "You are a helpful assistant."},
	})

	a2aMsg, err := ConvertToA2AMessage(msg)
	require.NoError(t, err)
	// A2A doesn't have a system role, so SystemMessage is converted to user role
	assert.Equal(t, a2atypes.MessageRole("user"), a2aMsg.Role)
	assert.Len(t, a2aMsg.Parts, 1)

	textPart, ok := a2aMsg.Parts[0].(a2atypes.TextPart)
	require.True(t, ok)
	assert.Equal(t, "You are a helpful assistant.", textPart.Text)
}

func TestConvertToA2AMessage_HumanMessage(t *testing.T) {
	msg := message.NewHumanMessage([]message.Part{
		message.TextPart{Text: "Hello, world!"},
	})

	a2aMsg, err := ConvertToA2AMessage(msg)
	require.NoError(t, err)
	assert.Equal(t, a2atypes.MessageRole("user"), a2aMsg.Role)
	assert.Len(t, a2aMsg.Parts, 1)

	textPart, ok := a2aMsg.Parts[0].(a2atypes.TextPart)
	require.True(t, ok)
	assert.Equal(t, "Hello, world!", textPart.Text)
}

func TestConvertToA2AMessage_NilMessage(t *testing.T) {
	_, err := ConvertToA2AMessage(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestConvertFromA2AMessage_UserMessage(t *testing.T) {
	a2aMsg := a2atypes.NewMessage("user", a2atypes.TextPart{Text: "Hello from user"})

	messages, err := ConvertFromA2AMessage(a2aMsg)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	humanMsg, ok := messages[0].(*message.HumanMessage)
	require.True(t, ok)
	parts := humanMsg.Parts()
	require.Len(t, parts, 1)

	textPart, ok := parts[0].(message.TextPart)
	require.True(t, ok)
	assert.Equal(t, "Hello from user", textPart.Text)
}

func TestConvertFromA2AMessage_NilMessage(t *testing.T) {
	_, err := ConvertFromA2AMessage(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestConvertMessagesToA2A(t *testing.T) {
	messages := []message.Message{
		message.NewSystemMessage([]message.Part{message.TextPart{Text: "System"}}),
		message.NewHumanMessage([]message.Part{message.TextPart{Text: "Human"}}),
		message.NewAIMessage([]message.Part{message.TextPart{Text: "AI"}}),
	}

	a2aMessages, err := ConvertMessagesToA2A(messages)
	require.NoError(t, err)
	assert.Len(t, a2aMessages, 3)

	// A2A doesn't have a system role, so SystemMessage is converted to user role
	assert.Equal(t, a2atypes.MessageRole("user"), a2aMessages[0].Role)
	assert.Equal(t, a2atypes.MessageRole("user"), a2aMessages[1].Role)
	assert.Equal(t, a2atypes.MessageRole("agent"), a2aMessages[2].Role)
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name     string
		msg      *a2atypes.Message
		expected string
	}{
		{
			name:     "nil message",
			msg:      nil,
			expected: "",
		},
		{
			name:     "single text part",
			msg:      a2atypes.NewMessage("user", a2atypes.TextPart{Text: "Hello"}),
			expected: "Hello",
		},
		{
			name: "multiple text parts",
			msg: a2atypes.NewMessage("user",
				a2atypes.TextPart{Text: "Hello"},
				a2atypes.TextPart{Text: " world"},
			),
			expected: "Hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTextContent(tt.msg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewClient(t *testing.T) {
	ctx := context.Background()

	// Test with empty URL
	_, err := NewClient(ctx, "", "skill-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agentCardURL cannot be empty")

	// Test with empty skill ID
	_, err = NewClient(ctx, "https://example.com", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "skillID cannot be empty")

	// Test with invalid URL (will fail to resolve)
	_, err = NewClient(ctx, "invalid-url", "test-skill")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve agent card")
}

func TestClient_SkillID(t *testing.T) {
	// Create a mock client with known skill ID
	client := &Client{
		skillID: "test-skill-123",
	}

	assert.Equal(t, "test-skill-123", client.SkillID())
}

func TestClient_Card(t *testing.T) {
	// Create a mock client with an agent card
	testCard := &a2atypes.AgentCard{
		Name: "TestAgent",
	}

	client := &Client{
		card: testCard,
	}

	result := client.Card()
	assert.Equal(t, testCard, result)
	assert.Equal(t, "TestAgent", result.Name)
}
