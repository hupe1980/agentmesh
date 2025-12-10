package langchaingo

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// mockLLM implements llms.Model for testing.
type mockLLM struct {
	generateContentFunc func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error)
	callFunc            func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error)
}

func (m *mockLLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.generateContentFunc != nil {
		return m.generateContentFunc(ctx, messages, options...)
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "default response"},
		},
	}, nil
}

func (m *mockLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	if m.callFunc != nil {
		return m.callFunc(ctx, prompt, options...)
	}
	return "default response", nil
}

func TestNewModel(t *testing.T) {
	t.Run("success with default options", func(t *testing.T) {
		mock := &mockLLM{}
		m, err := NewModel(mock)

		require.NoError(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0.7, m.opts.Temperature)
		assert.Equal(t, 4096, m.opts.MaxTokens)
		assert.False(t, m.opts.Streaming)
	})

	t.Run("success with custom options", func(t *testing.T) {
		mock := &mockLLM{}
		m, err := NewModel(mock,
			WithTemperature(0.5),
			WithMaxTokens(2048),
			WithStreaming(true),
			WithStopWords("stop1", "stop2"),
		)

		require.NoError(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0.5, m.opts.Temperature)
		assert.Equal(t, 2048, m.opts.MaxTokens)
		assert.True(t, m.opts.Streaming)
		assert.Equal(t, []string{"stop1", "stop2"}, m.opts.StopWords)
	})

	t.Run("error with nil llm", func(t *testing.T) {
		m, err := NewModel(nil)

		assert.Error(t, err)
		assert.Nil(t, m)
		assert.Contains(t, err.Error(), "langchaingo: llm")
	})
}

func TestMustNewModel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockLLM{}
		m := MustNewModel(mock)
		assert.NotNil(t, m)
	})

	t.Run("panics with nil llm", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNewModel(nil)
		})
	})
}

// mockToolImpl implements tool.Tool for testing.
type mockToolImpl struct {
	name        string
	description string
}

func (t *mockToolImpl) Name() string        { return t.name }
func (t *mockToolImpl) Description() string { return t.description }
func (t *mockToolImpl) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.name,
			Description: t.description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (t *mockToolImpl) Call(ctx context.Context, args string) (any, error) {
	return "mock result", nil
}

func TestModel_BindTools(t *testing.T) {
	mock := &mockLLM{}
	m, _ := NewModel(mock)

	mockTool := &mockToolImpl{
		name:        "test_tool",
		description: "A test tool",
	}

	bound := m.BindTools(mockTool)

	assert.NotNil(t, bound)
	assert.Len(t, bound.tools, 1)
	assert.Equal(t, "test_tool", bound.tools[0].Name())
	// Original model should be unchanged
	assert.Len(t, m.tools, 0)
}

func TestModel_Capabilities(t *testing.T) {
	mock := &mockLLM{}

	t.Run("default capabilities", func(t *testing.T) {
		m, _ := NewModel(mock)
		caps := m.Capabilities()

		assert.False(t, caps.Streaming)
		assert.True(t, caps.Tools)
		assert.False(t, caps.StructuredOutput)
		assert.False(t, caps.NativeReasoning)
		assert.False(t, caps.Logprobs)
		assert.False(t, caps.Vision)
		assert.False(t, caps.Audio)
		assert.Equal(t, 4096, caps.MaxOutputTokens)
		assert.Equal(t, []string{"text"}, caps.SupportedModalities)
	})

	t.Run("streaming enabled", func(t *testing.T) {
		m, _ := NewModel(mock, WithStreaming(true))
		caps := m.Capabilities()

		assert.True(t, caps.Streaming)
	})
}

func TestModel_Generate(t *testing.T) {
	t.Run("simple text response", func(t *testing.T) {
		mock := &mockLLM{
			generateContentFunc: func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				return &llms.ContentResponse{
					Choices: []*llms.ContentChoice{
						{
							Content:    "Hello, world!",
							StopReason: "stop",
						},
					},
				}, nil
			},
		}

		m, _ := NewModel(mock)
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Hi"),
			},
		}

		var responses []*model.Response
		for resp, err := range m.Generate(context.Background(), req) {
			require.NoError(t, err)
			responses = append(responses, resp)
		}

		require.Len(t, responses, 1)
		assert.Equal(t, "stop", responses[0].FinishReason)
		assert.False(t, responses[0].Partial)

		content := responses[0].Message.String()
		assert.Equal(t, "Hello, world!", content)
	})

	t.Run("with system prompt", func(t *testing.T) {
		var capturedMessages []llms.MessageContent
		mock := &mockLLM{
			generateContentFunc: func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				capturedMessages = messages
				return &llms.ContentResponse{
					Choices: []*llms.ContentChoice{
						{Content: "Response"},
					},
				}, nil
			},
		}

		m, _ := NewModel(mock)
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
			SystemPrompt: "You are a helpful assistant.",
		}

		for _, err := range m.Generate(context.Background(), req) {
			require.NoError(t, err)
		}

		require.Len(t, capturedMessages, 2)
		assert.Equal(t, llms.ChatMessageTypeSystem, capturedMessages[0].Role)
		assert.Equal(t, llms.ChatMessageTypeHuman, capturedMessages[1].Role)
	})

	t.Run("with tool calls", func(t *testing.T) {
		mock := &mockLLM{
			generateContentFunc: func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				return &llms.ContentResponse{
					Choices: []*llms.ContentChoice{
						{
							Content:    "",
							StopReason: "tool_calls",
							ToolCalls: []llms.ToolCall{
								{
									ID:   "call_123",
									Type: "function",
									FunctionCall: &llms.FunctionCall{
										Name:      "get_weather",
										Arguments: `{"location": "Berlin"}`,
									},
								},
							},
						},
					},
				}, nil
			},
		}

		m, _ := NewModel(mock)
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("What's the weather in Berlin?"),
			},
		}

		var responses []*model.Response
		for resp, err := range m.Generate(context.Background(), req) {
			require.NoError(t, err)
			responses = append(responses, resp)
		}

		require.Len(t, responses, 1)
		parts := responses[0].Message.Parts()
		require.Len(t, parts, 1)

		fcPart, ok := parts[0].(message.FunctionCallPart)
		require.True(t, ok)
		assert.Equal(t, "call_123", fcPart.FunctionCall.ID)
		assert.Equal(t, "get_weather", fcPart.FunctionCall.Name)
		assert.Equal(t, `{"location": "Berlin"}`, fcPart.FunctionCall.Arguments)

		// Verify ToolCalls is also populated on AIMessage
		aiMsg, ok := responses[0].Message.(*message.AIMessage)
		require.True(t, ok)
		require.Len(t, aiMsg.ToolCalls, 1)
		assert.Equal(t, "call_123", aiMsg.ToolCalls[0].ID)
		assert.Equal(t, "get_weather", aiMsg.ToolCalls[0].Name)
		assert.Equal(t, `{"location": "Berlin"}`, aiMsg.ToolCalls[0].Arguments)
	})

	t.Run("with reasoning content", func(t *testing.T) {
		mock := &mockLLM{
			generateContentFunc: func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				return &llms.ContentResponse{
					Choices: []*llms.ContentChoice{
						{
							Content:          "The answer is 42.",
							ReasoningContent: "Let me think about this...",
							StopReason:       "stop",
						},
					},
				}, nil
			},
		}

		m, _ := NewModel(mock)
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("What is the answer?"),
			},
		}

		var responses []*model.Response
		for resp, err := range m.Generate(context.Background(), req) {
			require.NoError(t, err)
			responses = append(responses, resp)
		}

		require.Len(t, responses, 1)
		assert.Equal(t, "Let me think about this...", responses[0].Reasoning)
	})

	t.Run("error on nil request", func(t *testing.T) {
		mock := &mockLLM{}
		m, _ := NewModel(mock)

		var gotErr error
		for _, err := range m.Generate(context.Background(), nil) {
			gotErr = err
		}

		assert.Error(t, gotErr)
		assert.Contains(t, gotErr.Error(), "requires at least one message")
	})

	t.Run("error on empty messages", func(t *testing.T) {
		mock := &mockLLM{}
		m, _ := NewModel(mock)

		req := &model.Request{
			Messages: []message.Message{},
		}

		var gotErr error
		for _, err := range m.Generate(context.Background(), req) {
			gotErr = err
		}

		assert.Error(t, gotErr)
		assert.Contains(t, gotErr.Error(), "requires at least one message")
	})

	t.Run("error from LLM", func(t *testing.T) {
		mock := &mockLLM{
			generateContentFunc: func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				return nil, errors.New("API error")
			},
		}

		m, _ := NewModel(mock)
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Hi"),
			},
		}

		var gotErr error
		for _, err := range m.Generate(context.Background(), req) {
			gotErr = err
		}

		assert.Error(t, gotErr)
		assert.Contains(t, gotErr.Error(), "generation failed")
		assert.Contains(t, gotErr.Error(), "API error")
	})

	t.Run("error on empty response", func(t *testing.T) {
		mock := &mockLLM{
			generateContentFunc: func(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				return &llms.ContentResponse{
					Choices: []*llms.ContentChoice{},
				}, nil
			},
		}

		m, _ := NewModel(mock)
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Hi"),
			},
		}

		var gotErr error
		for _, err := range m.Generate(context.Background(), req) {
			gotErr = err
		}

		assert.Error(t, gotErr)
		assert.Contains(t, gotErr.Error(), "failed to convert response")
	})
}

func TestModel_ConvertRole(t *testing.T) {
	mock := &mockLLM{}
	m, _ := NewModel(mock)

	tests := []struct {
		input    message.Type
		expected llms.ChatMessageType
	}{
		{message.TypeSystem, llms.ChatMessageTypeSystem},
		{message.TypeHuman, llms.ChatMessageTypeHuman},
		{message.TypeAI, llms.ChatMessageTypeAI},
		{message.TypeTool, llms.ChatMessageTypeTool},
		{message.TypeFunction, llms.ChatMessageTypeFunction},
		{message.Type("unknown"), llms.ChatMessageTypeHuman}, // Default
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := m.convertRole(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModel_ConvertParts(t *testing.T) {
	mock := &mockLLM{}
	m, _ := NewModel(mock)

	t.Run("text part", func(t *testing.T) {
		parts := []message.Part{
			message.TextPart{Text: "Hello"},
		}

		result := m.convertParts(parts)

		require.Len(t, result, 1)
		textContent, ok := result[0].(llms.TextContent)
		require.True(t, ok)
		assert.Equal(t, "Hello", textContent.Text)
	})

	t.Run("function call part", func(t *testing.T) {
		parts := []message.Part{
			message.FunctionCallPart{
				FunctionCall: &message.FunctionCall{
					ID:        "call_1",
					Name:      "test_func",
					Arguments: `{"arg": "value"}`,
				},
			},
		}

		result := m.convertParts(parts)

		require.Len(t, result, 1)
		toolCall, ok := result[0].(llms.ToolCall)
		require.True(t, ok)
		assert.Equal(t, "call_1", toolCall.ID)
		assert.Equal(t, "function", toolCall.Type)
		assert.Equal(t, "test_func", toolCall.FunctionCall.Name)
		assert.Equal(t, `{"arg": "value"}`, toolCall.FunctionCall.Arguments)
	})

	t.Run("function response part", func(t *testing.T) {
		parts := []message.Part{
			message.FunctionResponsePart{
				FunctionResponse: &message.FunctionResponse{
					ID:       "call_1",
					Name:     "test_func",
					Response: "result string",
				},
			},
		}

		result := m.convertParts(parts)

		require.Len(t, result, 1)
		toolResponse, ok := result[0].(llms.ToolCallResponse)
		require.True(t, ok)
		assert.Equal(t, "call_1", toolResponse.ToolCallID)
		assert.Equal(t, "test_func", toolResponse.Name)
		assert.Equal(t, "result string", toolResponse.Content)
	})
}

func TestModel_ConvertTools(t *testing.T) {
	mock := &mockLLM{}
	m, _ := NewModel(mock)

	mockTool := &mockToolImpl{
		name:        "test_tool",
		description: "A test tool",
	}

	result := m.convertTools([]tool.Tool{mockTool})

	require.Len(t, result, 1)
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "test_tool", result[0].Function.Name)
	assert.Equal(t, "A test tool", result[0].Function.Description)
	assert.NotNil(t, result[0].Function.Parameters)
}
