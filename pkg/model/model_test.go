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
	response *model.Response
	err      error
}

func (m *mockModel) Generate(ctx context.Context, messages []message.Message) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		if m.err != nil {
			yield(nil, m.err)
			return
		}

		// Simulate streaming with intermediate chunks
		parts := m.response.Message.Parts()
		if len(parts) > 0 {
			if textPart, ok := parts[0].(message.TextPart); ok {
				// Yield intermediate chunks
				text := textPart.Text
				if len(text) > 3 {
					chunk1 := message.NewAIMessageFromText(text[:len(text)/2])
					chunkResp := &model.Response{
						Message: chunk1,
					}
					if !yield(chunkResp, nil) {
						return
					}
				}
			}
		}

		// Yield final response
		yield(m.response, nil)
	}
}

// TestMockModel_Generate verifies mock model implementation
func TestMockModel_Generate(t *testing.T) {
	expectedMsg := message.NewAIMessageFromText("Test response")
	mock := &mockModel{
		response: &model.Response{
			Message: expectedMsg,
			Usage: &model.UsageInfo{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		},
	}

	result, err := model.Last(mock.Generate(context.Background(), nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	parts := result.Message.Parts()
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

	// Verify usage information
	if result.Usage == nil {
		t.Fatal("Expected usage information")
	}
	if result.Usage.TotalTokens != 15 {
		t.Errorf("Expected 15 total tokens, got %d", result.Usage.TotalTokens)
	}
}

// TestMockModel_Stream verifies mock streaming
func TestMockModel_Stream(t *testing.T) {
	expectedMsg := message.NewAIMessageFromText("Streaming")
	mock := &mockModel{
		response: &model.Response{
			Message: expectedMsg,
		},
	}

	// Test streaming by collecting all responses
	responses, err := model.Collect(mock.Generate(context.Background(), nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(responses) < 1 {
		t.Fatal("Expected at least one response")
	}

	// Verify the final response
	finalResp := responses[len(responses)-1]
	parts := finalResp.Message.Parts()
	if len(parts) == 0 {
		t.Fatal("Expected final message to have parts")
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		if textPart.Text != "Streaming" {
			t.Errorf("Expected 'Streaming', got %q", textPart.Text)
		}
	}
}
