package model_test

import (
	"context"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	parts := result.Message.Parts()
	require.NotEmpty(t, parts, "Expected message to have parts")

	textPart, ok := parts[0].(message.TextPart)
	require.True(t, ok, "Expected first part to be TextPart")
	assert.Equal(t, "Test response", textPart.Text)

	// Verify usage information
	require.NotNil(t, result.Usage, "Expected usage information")
	assert.Equal(t, 15, result.Usage.TotalTokens)
	assert.Equal(t, 10, result.Usage.PromptTokens)
	assert.Equal(t, 5, result.Usage.CompletionTokens)
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
	require.NoError(t, err)
	require.NotEmpty(t, responses, "Expected at least one response")

	// Verify the final response
	finalResp := responses[len(responses)-1]
	parts := finalResp.Message.Parts()
	require.NotEmpty(t, parts, "Expected final message to have parts")

	textPart, ok := parts[0].(message.TextPart)
	require.True(t, ok, "Expected first part to be TextPart")
	assert.Equal(t, "Streaming", textPart.Text)
}
