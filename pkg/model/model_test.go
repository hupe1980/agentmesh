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
	response     *model.Response
	err          error
	capabilities model.Capabilities
}

func (m *mockModel) Capabilities() model.Capabilities {
	if m.capabilities.MaxContextTokens == 0 {
		// Return default capabilities if not set
		return model.Capabilities{
			Streaming:           true,
			Tools:               false,
			StructuredOutput:    false,
			MaxContextTokens:    4096,
			MaxOutputTokens:     2048,
			SupportedModalities: []string{"text"},
		}
	}
	return m.capabilities
}

func (m *mockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
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
						Partial: true, // Streaming chunk
					}
					if !yield(chunkResp, nil) {
						return
					}
				}
			}
		}

		// Yield final response (make a copy with Partial=false)
		finalResp := &model.Response{
			Message:      m.response.Message,
			Reasoning:    m.response.Reasoning,
			FinishReason: m.response.FinishReason,
			Logprobs:     m.response.Logprobs,
			Usage:        m.response.Usage,
			Metadata:     m.response.Metadata,
			Partial:      false, // Final complete response
		}
		yield(finalResp, nil)
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

	req := &model.Request{Messages: []message.Message{message.NewHumanMessageFromText("test")}}
	result, err := model.Last(mock.Generate(context.Background(), req))
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
	req := &model.Request{Messages: []message.Message{message.NewHumanMessageFromText("test")}}
	responses, err := model.Collect(mock.Generate(context.Background(), req))
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
