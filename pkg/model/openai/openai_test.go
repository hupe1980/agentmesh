package openai

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/openai/openai-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockClient is a simple mock implementation of the Client interface
type mockClient struct {
	mock.Mock
}

func (m *mockClient) ChatCompletions(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openai.ChatCompletion), args.Error(1)
}

func (m *mockClient) ChatCompletionsStreaming(ctx context.Context, req openai.ChatCompletionNewParams) Stream {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(Stream)
}

func TestNewModelFromClientWrapper(t *testing.T) {
	t.Run("creates model with default options", func(t *testing.T) {
		wrapper := &ClientWrapper{inner: nil}

		mdl, err := NewModelFromClientWrapper(wrapper)

		require.NoError(t, err)
		assert.NotNil(t, mdl)
		assert.Equal(t, openai.ChatModelGPT4oMini, mdl.model)
		assert.Equal(t, 0.7, mdl.opts.temperature)
		assert.Equal(t, int64(4096), mdl.opts.maxCompletionTokens)
	})

	t.Run("creates model with custom options", func(t *testing.T) {
		wrapper := &ClientWrapper{inner: nil}

		mdl, err := NewModelFromClientWrapper(wrapper,
			WithModel(openai.ChatModelGPT4o),
			WithTemperature(0.5),
			WithMaxCompletionTokens(2048),
		)

		require.NoError(t, err)
		assert.NotNil(t, mdl)
		assert.Equal(t, openai.ChatModelGPT4o, mdl.model)
		assert.Equal(t, 0.5, mdl.opts.temperature)
		assert.Equal(t, int64(2048), mdl.opts.maxCompletionTokens)
	})

	t.Run("returns error for nil wrapper", func(t *testing.T) {
		mdl, err := NewModelFromClientWrapper(nil)

		assert.Error(t, err)
		assert.Nil(t, mdl)
		assert.Contains(t, err.Error(), "wrapper cannot be nil")
	})
}

func TestModel_Name(t *testing.T) {
	wrapper := &ClientWrapper{inner: nil}
	mdl, _ := NewModelFromClientWrapper(wrapper, WithModel("gpt-4o"))

	assert.Equal(t, "gpt-4o", mdl.Name())
}

func TestModel_Capabilities(t *testing.T) {
	tests := []struct {
		name               string
		modelName          string
		expectedStreaming  bool
		expectedTools      bool
		expectedReasoning  bool
		expectedVision     bool
		expectedContextWin int
	}{
		{
			name:               "GPT-4o supports all features",
			modelName:          openai.ChatModelGPT4o,
			expectedStreaming:  true,
			expectedTools:      true,
			expectedReasoning:  false,
			expectedVision:     true,
			expectedContextWin: 128000,
		},
		{
			name:               "GPT-4o-mini supports all features",
			modelName:          openai.ChatModelGPT4oMini,
			expectedStreaming:  true,
			expectedTools:      true,
			expectedReasoning:  false,
			expectedVision:     true,
			expectedContextWin: 128000,
		},
		{
			name:               "O1-preview is reasoning model without tools",
			modelName:          openai.ChatModelO1Preview,
			expectedStreaming:  true,
			expectedTools:      false,
			expectedReasoning:  true,
			expectedVision:     false,
			expectedContextWin: 128000,
		},
		{
			name:               "GPT-3.5-turbo has limited capabilities",
			modelName:          "gpt-3.5-turbo",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedReasoning:  false,
			expectedVision:     false,
			expectedContextWin: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := &ClientWrapper{inner: nil}
			mdl, err := NewModelFromClientWrapper(wrapper, WithModel(tt.modelName))
			require.NoError(t, err)

			caps := mdl.Capabilities()

			assert.Equal(t, tt.expectedStreaming, caps.Streaming, "Streaming capability mismatch")
			assert.Equal(t, tt.expectedTools, caps.Tools, "Tools capability mismatch")
			assert.Equal(t, tt.expectedReasoning, caps.NativeReasoning, "Reasoning capability mismatch")
			assert.Equal(t, tt.expectedVision, caps.Vision, "Vision capability mismatch")
			assert.Equal(t, tt.expectedContextWin, caps.MaxContextTokens, "Context window mismatch")
		})
	}
}

func TestModel_Generate_Success(t *testing.T) {
	mockCli := new(mockClient)
	mdl := &Model{
		client: mockCli,
		model:  openai.ChatModelGPT4oMini,
		opts:   Options{temperature: 0.7, maxCompletionTokens: 4096},
	}

	// Mock response
	expectedResponse := &openai.ChatCompletion{
		ID:      "chatcmpl-123",
		Created: 1234567890,
		Model:   openai.ChatModelGPT4oMini,
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Content: "Hello! How can I help you today?",
				},
				FinishReason: "stop",
			},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 8,
			TotalTokens:      18,
		},
	}

	mockCli.On("ChatCompletions", mock.Anything, mock.Anything).Return(expectedResponse, nil)

	ctx := context.Background()
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Hello"),
		},
	}

	// Execute and collect responses
	var responses []*model.Response
	for resp, err := range mdl.Generate(ctx, req) {
		require.NoError(t, err)
		responses = append(responses, resp)
	}

	// Assert
	require.Len(t, responses, 1)
	resp := responses[0]

	// Extract text from message parts
	parts := resp.Message.Parts()
	require.Len(t, parts, 1)
	textPart, ok := parts[0].(message.TextPart)
	require.True(t, ok, "Expected TextPart")
	assert.Equal(t, "Hello! How can I help you today?", textPart.Text)

	assert.NotNil(t, resp.Usage)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 8, resp.Usage.CompletionTokens)
	assert.Equal(t, 18, resp.Usage.TotalTokens)

	mockCli.AssertExpectations(t)
}

func TestModel_Generate_Error(t *testing.T) {
	mockCli := new(mockClient)
	mdl := &Model{
		client: mockCli,
		model:  openai.ChatModelGPT4oMini,
		opts:   Options{temperature: 0.7, maxCompletionTokens: 4096},
	}

	expectedError := errors.New("API error: rate limit exceeded")
	mockCli.On("ChatCompletions", mock.Anything, mock.Anything).Return(nil, expectedError)

	ctx := context.Background()
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Hello"),
		},
	}

	// Execute
	var count int
	var gotError error
	for _, err := range mdl.Generate(ctx, req) {
		count++
		if err != nil {
			gotError = err
		}
	}

	assert.Equal(t, 1, count, "Should yield exactly one error")
	assert.Error(t, gotError)
	assert.Contains(t, gotError.Error(), "rate limit exceeded")

	mockCli.AssertExpectations(t)
}

func TestModel_Capabilities_ContextWindows(t *testing.T) {
	tests := []struct {
		modelName      string
		expectedTokens int
	}{
		{openai.ChatModelGPT4o, 128000},
		{openai.ChatModelGPT4oMini, 128000},
		{openai.ChatModelO1Preview, 128000},
		{"gpt-3.5-turbo", 4096},
		{"gpt-4-turbo", 128000},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			wrapper := &ClientWrapper{inner: nil}
			mdl, err := NewModelFromClientWrapper(wrapper, WithModel(tt.modelName))
			require.NoError(t, err)

			caps := mdl.Capabilities()
			assert.Equal(t, tt.expectedTokens, caps.MaxContextTokens)
		})
	}
}

func TestNewClientWrapper(t *testing.T) {
	t.Run("creates wrapper successfully", func(t *testing.T) {
		client := openai.NewClient()
		wrapper, err := NewClientWrapper(&client)

		require.NoError(t, err)
		assert.NotNil(t, wrapper)
		assert.Equal(t, &client, wrapper.inner)
	})

	t.Run("returns error for nil client", func(t *testing.T) {
		wrapper, err := NewClientWrapper(nil)

		assert.Error(t, err)
		assert.Nil(t, wrapper)
		assert.Contains(t, err.Error(), "client cannot be nil")
	})
}
