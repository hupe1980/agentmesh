package ollama

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockClient implements the Client interface for testing.
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	args := m.Called(ctx, req, fn)
	if args.Get(0) != nil {
		return args.Error(0)
	}
	// Simulate response
	if fn != nil {
		fn(api.ChatResponse{
			Message: api.Message{
				Role:    "assistant",
				Content: "Hello from mock!",
			},
			Done: true,
			Metrics: api.Metrics{
				PromptEvalCount: 10,
				EvalCount:       5,
			},
		})
	}
	return nil
}

func (m *MockClient) Generate(ctx context.Context, req *api.GenerateRequest, fn api.GenerateResponseFunc) error {
	args := m.Called(ctx, req, fn)
	return args.Error(0)
}

func TestNewModel(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)
	assert.Equal(t, "llama3.2", m.Name())
}

func TestNewModelWithOptions(t *testing.T) {
	m := NewModel(
		WithModel("mistral"),
		WithTemperature(0.5),
		WithNumPredict(200),
		WithTopK(50),
		WithTopP(0.95),
		WithSeed(123),
	)

	require.NotNil(t, m)
	assert.Equal(t, "mistral", m.Name())
	assert.Equal(t, 0.5, m.opts.temperature)
	assert.Equal(t, 200, m.opts.numPredict)
	assert.Equal(t, 50, m.opts.topK)
	assert.Equal(t, 0.95, m.opts.topP)
	assert.Equal(t, 123, m.opts.seed)
}

func TestCapabilities(t *testing.T) {
	m := NewModel()
	caps := m.Capabilities()

	assert.True(t, caps.Streaming, "Should support streaming")
	assert.True(t, caps.Tools, "Should support tools")
	assert.False(t, caps.StructuredOutput, "Should not support structured output")
	assert.False(t, caps.NativeReasoning, "Should not support native reasoning")
}

func TestGenerate_NonStreaming(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	m := &Model{
		client: mockClient,
		model:  "test-model",
		opts: Options{
			temperature: 0.7,
			numPredict:  -1,
			topK:        40,
			topP:        0.9,
			seed:        -1,
		},
	}

	req := model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Hello"),
		},
		Stream: false,
	}

	var responses []model.Response
	for resp, err := range m.Generate(context.Background(), req) {
		require.NoError(t, err)
		responses = append(responses, resp)
	}

	assert.Len(t, responses, 1)
	assert.False(t, responses[0].Partial)
	assert.Equal(t, "stop", responses[0].FinishReason)

	// Verify message content
	parts := responses[0].Message.Parts()
	require.Len(t, parts, 1)
	textPart, ok := parts[0].(message.TextPart)
	require.True(t, ok)
	assert.Equal(t, "Hello from mock!", textPart.Text)

	mockClient.AssertExpectations(t)
}

func TestGenerate_Streaming(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Chat", mock.Anything, mock.Anything, mock.MatchedBy(func(fn api.ChatResponseFunc) bool {
		// Simulate streaming responses
		fn(api.ChatResponse{
			Message: api.Message{
				Role:    "assistant",
				Content: "Hello ",
			},
			Done: false,
		})
		fn(api.ChatResponse{
			Message: api.Message{
				Role:    "assistant",
				Content: "world!",
			},
			Done: true,
			Metrics: api.Metrics{
				PromptEvalCount: 5,
				EvalCount:       2,
			},
		})
		return true
	})).Return(nil)

	m := &Model{
		client: mockClient,
		model:  "test-model",
		opts: Options{
			temperature: 0.7,
			numPredict:  -1,
			topK:        40,
			topP:        0.9,
			seed:        -1,
		},
	}

	req := model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Say hello"),
		},
		Stream: true,
	}

	var responses []model.Response
	for resp, err := range m.Generate(context.Background(), req) {
		require.NoError(t, err)
		responses = append(responses, resp)
	}

	// Should have partial chunks and final response
	assert.GreaterOrEqual(t, len(responses), 2)

	// Check last response is final
	lastResp := responses[len(responses)-1]
	assert.False(t, lastResp.Partial)
	assert.Equal(t, "stop", lastResp.FinishReason)
	assert.NotNil(t, lastResp.Usage)

	mockClient.AssertExpectations(t)
}

func TestConvertMessages(t *testing.T) {
	m := NewModel()

	messages := []message.Message{
		message.NewSystemMessageFromText("You are helpful"),
		message.NewHumanMessageFromText("Hi"),
		message.NewAIMessageFromText("Hello!"),
	}

	converted, err := m.convertMessages(messages, "")
	require.NoError(t, err)
	require.Len(t, converted, 3)

	assert.Equal(t, "system", converted[0].Role)
	assert.Equal(t, "You are helpful", converted[0].Content)

	assert.Equal(t, "user", converted[1].Role)
	assert.Equal(t, "Hi", converted[1].Content)

	assert.Equal(t, "assistant", converted[2].Role)
	assert.Equal(t, "Hello!", converted[2].Content)
}

func TestConvertMessages_WithSystemPrompt(t *testing.T) {
	m := NewModel()

	messages := []message.Message{
		message.NewHumanMessageFromText("What's the weather?"),
	}

	converted, err := m.convertMessages(messages, "You are a weather assistant")
	require.NoError(t, err)
	require.Len(t, converted, 2)

	assert.Equal(t, "system", converted[0].Role)
	assert.Equal(t, "You are a weather assistant", converted[0].Content)

	assert.Equal(t, "user", converted[1].Role)
	assert.Equal(t, "What's the weather?", converted[1].Content)
}

func TestConvertMessage_WithToolCalls(t *testing.T) {
	m := NewModel()

	aiMsg := message.NewAIMessageFromText("I'll check the weather")
	aiMsg.ToolCalls = []message.ToolCall{
		{
			ID:        "call_123",
			Name:      "get_weather",
			Type:      "function",
			Arguments: `{"location":"Paris"}`,
		},
	}

	converted, err := m.convertMessage(aiMsg)
	require.NoError(t, err)

	assert.Equal(t, "assistant", converted.Role)
	assert.Equal(t, "I'll check the weather", converted.Content)
	require.Len(t, converted.ToolCalls, 1)
	assert.Equal(t, "get_weather", converted.ToolCalls[0].Function.Name)
}

func TestExtractTextFromParts(t *testing.T) {
	m := NewModel()

	parts := message.Parts{
		message.TextPart{Text: "Hello "},
		message.TextPart{Text: "world!"},
		message.DataPart{Data: map[string]any{"key": "value"}}, // Should be ignored
	}

	text := m.extractTextFromParts(parts)
	assert.Equal(t, "Hello world!", text)
}
