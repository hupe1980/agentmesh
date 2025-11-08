package model_test

import (
	"context"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// mockModel is a test implementation for integration tests
type mockModel struct {
	response message.Message
	err      error
}

func (m *mockModel) Generate(ctx context.Context, messages []message.Message) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		if m.err != nil {
			yield(nil, m.err)
			return
		}

		// Simulate streaming with intermediate chunks
		parts := m.response.Parts()
		if len(parts) > 0 {
			if textPart, ok := parts[0].(message.TextPart); ok {
				// Yield intermediate chunks
				text := textPart.Text
				if len(text) > 3 {
					chunk1 := message.NewAIMessageFromText(text[:len(text)/2])
					if !yield(chunk1, nil) {
						return
					}
				}
			}
		}

		// Yield final message
		yield(m.response, nil)
	}
}

// TestMockModel_Generate verifies mock model implementation
func TestMockModel_Generate(t *testing.T) {
	expectedMsg := message.NewAIMessageFromText("Test response")
	mock := &mockModel{response: expectedMsg}

	result, err := model.Last(mock.Generate(context.Background(), nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	parts := result.Parts()
	if len(parts) == 0 {
		t.Fatal("Expected message to have parts")
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		if textPart.Text != "Test response" {
			t.Errorf("Expected 'Test response', got %q", textPart.Text)
		}
	} else {
		t.Error("Expected first part to be TextPart")
	}
}

// TestMockModel_Stream verifies mock streaming
func TestMockModel_Stream(t *testing.T) {
	expectedMsg := message.NewAIMessageFromText("Streaming")
	mock := &mockModel{response: expectedMsg}

	// Test streaming by collecting all messages
	messages, err := model.Collect(mock.Generate(context.Background(), nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(messages) < 1 {
		t.Fatal("Expected at least one message")
	}

	// Verify the final message
	finalMsg := messages[len(messages)-1]
	parts := finalMsg.Parts()
	if len(parts) == 0 {
		t.Fatal("Expected final message to have parts")
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		if textPart.Text != "Streaming" {
			t.Errorf("Expected 'Streaming', got %q", textPart.Text)
		}
	}
}
