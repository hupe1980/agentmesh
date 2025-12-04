package amazonbedrock

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockClient is a mock implementation of the Client interface.
type mockClient struct {
	mock.Mock
}

func (m *mockClient) Converse(
	ctx context.Context,
	params *bedrockruntime.ConverseInput,
	optFns ...func(*bedrockruntime.Options),
) (*bedrockruntime.ConverseOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bedrockruntime.ConverseOutput), args.Error(1)
}

func (m *mockClient) ConverseStream(
	ctx context.Context,
	params *bedrockruntime.ConverseStreamInput,
	optFns ...func(*bedrockruntime.Options),
) (*bedrockruntime.ConverseStreamOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bedrockruntime.ConverseStreamOutput), args.Error(1)
}

func TestNewModel(t *testing.T) {
	t.Run("creates model with default options", func(t *testing.T) {
		client := &mockClient{}
		mdl := NewModel(client)

		assert.NotNil(t, mdl)
		assert.Equal(t, DefaultModelID, mdl.opts.ModelID)
		assert.Equal(t, int32(4096), mdl.opts.MaxTokens)
	})

	t.Run("creates model with custom options", func(t *testing.T) {
		client := &mockClient{}
		mdl := NewModel(client,
			WithModelID("anthropic.claude-3-haiku-20240307-v1:0"),
			WithTemperature(0.5),
			WithMaxTokens(2048),
			WithTopP(0.9),
		)

		assert.NotNil(t, mdl)
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", mdl.opts.ModelID)
		assert.Equal(t, float32(0.5), mdl.opts.Temperature)
		assert.Equal(t, int32(2048), mdl.opts.MaxTokens)
		assert.Equal(t, float32(0.9), mdl.opts.TopP)
	})
}

func TestModel_Capabilities(t *testing.T) {
	tests := []struct {
		name               string
		modelID            string
		expectedStreaming  bool
		expectedTools      bool
		expectedVision     bool
		expectedContextWin int
	}{
		{
			name:               "Claude 3.5 Sonnet supports all features",
			modelID:            "anthropic.claude-3-5-sonnet-20241022-v2:0",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedVision:     true,
			expectedContextWin: 200000,
		},
		{
			name:               "Claude 3 Haiku supports all features",
			modelID:            "anthropic.claude-3-haiku-20240307-v1:0",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedVision:     true,
			expectedContextWin: 200000,
		},
		{
			name:               "Mistral Large supports tools but not vision",
			modelID:            "mistral.mistral-large-2402-v1:0",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedVision:     false,
			expectedContextWin: 128000,
		},
		{
			name:               "Llama 3 70B",
			modelID:            "meta.llama3-70b-instruct-v1:0",
			expectedStreaming:  true,
			expectedTools:      false,
			expectedVision:     false,
			expectedContextWin: 128000,
		},
		{
			name:               "Amazon Titan supports basic features",
			modelID:            "amazon.titan-text-premier-v1:0",
			expectedStreaming:  true,
			expectedTools:      false,
			expectedVision:     false,
			expectedContextWin: 32000,
		},
		{
			name:               "EU Nova Pro inference profile supports tools and vision",
			modelID:            "eu.amazon.nova-pro-v1:0",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedVision:     true,
			expectedContextWin: 100000,
		},
		{
			name:               "EU Claude inference profile supports tools and vision",
			modelID:            "eu.anthropic.claude-3-haiku-20240307-v1:0",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedVision:     true,
			expectedContextWin: 200000,
		},
		{
			name:               "EU Llama 3.2 inference profile supports tools",
			modelID:            "eu.meta.llama3-2-3b-instruct-v1:0",
			expectedStreaming:  true,
			expectedTools:      true,
			expectedVision:     false,
			expectedContextWin: 100000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{}
			mdl := NewModel(client, WithModelID(tt.modelID))

			caps := mdl.Capabilities()

			assert.Equal(t, tt.expectedStreaming, caps.Streaming)
			assert.Equal(t, tt.expectedTools, caps.Tools)
			assert.Equal(t, tt.expectedVision, caps.Vision)
			assert.Equal(t, tt.expectedContextWin, caps.MaxContextTokens)
		})
	}
}

func TestModel_Generate(t *testing.T) {
	t.Run("returns error for nil request", func(t *testing.T) {
		client := &mockClient{}
		mdl := NewModel(client)

		var lastErr error
		for _, err := range mdl.Generate(context.Background(), nil) {
			lastErr = err
		}

		assert.Error(t, lastErr)
		assert.Contains(t, lastErr.Error(), "requires at least one message")
	})

	t.Run("returns error for empty messages", func(t *testing.T) {
		client := &mockClient{}
		mdl := NewModel(client)

		req := &model.Request{Messages: []message.Message{}}

		var lastErr error
		for _, err := range mdl.Generate(context.Background(), req) {
			lastErr = err
		}

		assert.Error(t, lastErr)
		assert.Contains(t, lastErr.Error(), "requires at least one message")
	})

	t.Run("successful non-streaming generation", func(t *testing.T) {
		client := &mockClient{}
		mdl := NewModel(client)

		expectedOutput := &bedrockruntime.ConverseOutput{
			Output: &types.ConverseOutputMemberMessage{
				Value: types.Message{
					Role: types.ConversationRoleAssistant,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberText{Value: "Hello, world!"},
					},
				},
			},
			StopReason: types.StopReasonEndTurn,
			Usage: &types.TokenUsage{
				InputTokens:  aws.Int32(10),
				OutputTokens: aws.Int32(5),
				TotalTokens:  aws.Int32(15),
			},
		}

		client.On("Converse", mock.Anything, mock.Anything).Return(expectedOutput, nil)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		}

		var responses []*model.Response
		for resp, err := range mdl.Generate(context.Background(), req) {
			require.NoError(t, err)
			responses = append(responses, resp)
		}

		require.Len(t, responses, 1)
		resp := responses[0]
		assert.Equal(t, "end_turn", resp.FinishReason)
		assert.Equal(t, 10, resp.Usage.PromptTokens)
		assert.Equal(t, 5, resp.Usage.CompletionTokens)
		assert.Equal(t, 15, resp.Usage.TotalTokens)
		assert.False(t, resp.Partial)
		assert.Equal(t, "Hello, world!", message.Stringify(resp.Message))
	})

	t.Run("generation with tool calls", func(t *testing.T) {
		client := &mockClient{}
		mdl := NewModel(client)

		expectedOutput := &bedrockruntime.ConverseOutput{
			Output: &types.ConverseOutputMemberMessage{
				Value: types.Message{
					Role: types.ConversationRoleAssistant,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberToolUse{
							Value: types.ToolUseBlock{
								ToolUseId: aws.String("call_123"),
								Name:      aws.String("get_weather"),
								Input:     document.NewLazyDocument(map[string]any{"location": "Berlin"}),
							},
						},
					},
				},
			},
			StopReason: types.StopReasonToolUse,
		}

		client.On("Converse", mock.Anything, mock.Anything).Return(expectedOutput, nil)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("What's the weather in Berlin?"),
			},
		}

		var responses []*model.Response
		for resp, err := range mdl.Generate(context.Background(), req) {
			require.NoError(t, err)
			responses = append(responses, resp)
		}

		require.Len(t, responses, 1)
		resp := responses[0]

		aiMsg, ok := resp.Message.(*message.AIMessage)
		require.True(t, ok)
		require.Len(t, aiMsg.ToolCalls, 1)

		tc := aiMsg.ToolCalls[0]
		assert.Equal(t, "call_123", tc.ID)
		assert.Equal(t, "get_weather", tc.Name)
		assert.Contains(t, tc.Arguments, "Berlin")
	})
}

func TestConvertMessagesToBedrock(t *testing.T) {
	t.Run("converts human message", func(t *testing.T) {
		msgs := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		result, systemPrompt := convertMessagesToBedrock(msgs)

		assert.Empty(t, systemPrompt)
		require.Len(t, result, 1)
		assert.Equal(t, types.ConversationRoleUser, result[0].Role)
	})

	t.Run("extracts system message", func(t *testing.T) {
		msgs := []message.Message{
			message.NewSystemMessageFromText("You are a helpful assistant."),
			message.NewHumanMessageFromText("Hello"),
		}

		result, systemPrompt := convertMessagesToBedrock(msgs)

		assert.Equal(t, "You are a helpful assistant.", systemPrompt)
		require.Len(t, result, 1) // Only human message should be in result
	})

	t.Run("converts AI message with tool calls", func(t *testing.T) {
		aiMsg := message.NewAIMessageFromText("Let me check that")
		aiMsg.ToolCalls = []message.ToolCall{
			{
				ID:        "call_123",
				Name:      "get_weather",
				Type:      "function",
				Arguments: `{"location":"Berlin"}`,
			},
		}

		msgs := []message.Message{aiMsg}

		result, _ := convertMessagesToBedrock(msgs)

		require.Len(t, result, 1)
		assert.Equal(t, types.ConversationRoleAssistant, result[0].Role)
		require.Len(t, result[0].Content, 2) // Text + ToolUse
	})

	t.Run("converts tool result message", func(t *testing.T) {
		msgs := []message.Message{
			message.NewToolMessage("call_123", "The weather is sunny."),
		}

		result, _ := convertMessagesToBedrock(msgs)

		require.Len(t, result, 1)
		assert.Equal(t, types.ConversationRoleUser, result[0].Role)
	})
}

// Ensure Model implements model.Model interface
var _ model.Model = (*Model)(nil)
