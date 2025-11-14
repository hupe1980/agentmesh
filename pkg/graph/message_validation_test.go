package graph

import (
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageValidation_MaxMessageSize(t *testing.T) {
	// Create a simple graph
	builder, err := NewBuilder()
	require.NoError(t, err)

	builder.
		Node("start", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		}).
		AddEdge(StartNode, "start").
		AddEdge("start", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	tests := []struct {
		name        string
		messages    []message.Message
		maxSize     int
		expectError bool
		errorType   string
	}{
		{
			name: "message within limit",
			messages: []message.Message{
				message.NewHumanMessageFromText("short message"),
			},
			maxSize:     100,
			expectError: false,
		},
		{
			name: "message exceeds limit",
			messages: []message.Message{
				message.NewHumanMessageFromText(strings.Repeat("A", 1000)),
			},
			maxSize:     500,
			expectError: true,
			errorType:   "message_size",
		},
		{
			name: "multiple messages all within limit",
			messages: []message.Message{
				message.NewHumanMessageFromText("message 1"),
				message.NewHumanMessageFromText("message 2"),
				message.NewHumanMessageFromText("message 3"),
			},
			maxSize:     100,
			expectError: false,
		},
		{
			name: "second message exceeds limit",
			messages: []message.Message{
				message.NewHumanMessageFromText("short"),
				message.NewHumanMessageFromText(strings.Repeat("B", 1000)),
			},
			maxSize:     500,
			expectError: true,
			errorType:   "message_size",
		},
		{
			name: "no limit set",
			messages: []message.Message{
				message.NewHumanMessageFromText(strings.Repeat("C", 10000)),
			},
			maxSize:     0, // unlimited
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			seq := compiled.Run(ctx, tt.messages, WithMaxMessageSize(tt.maxSize))

			_, err := Last(seq)
			if tt.expectError {
				require.Error(t, err)
				var validationErr *MessageValidationError
				if assert.ErrorAs(t, err, &validationErr) {
					assert.Equal(t, tt.errorType, validationErr.Type)
					assert.True(t, errors.Is(validationErr, ErrMessageTooLarge))
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMessageValidation_MaxInputMessages(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.
		Node("start", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		}).
		AddEdge(StartNode, "start").
		AddEdge("start", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	tests := []struct {
		name        string
		numMessages int
		maxMessages int
		expectError bool
	}{
		{
			name:        "within limit",
			numMessages: 5,
			maxMessages: 10,
			expectError: false,
		},
		{
			name:        "at limit",
			numMessages: 10,
			maxMessages: 10,
			expectError: false,
		},
		{
			name:        "exceeds limit",
			numMessages: 15,
			maxMessages: 10,
			expectError: true,
		},
		{
			name:        "no limit",
			numMessages: 100,
			maxMessages: 0, // unlimited
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := make([]message.Message, tt.numMessages)
			for i := range messages {
				messages[i] = message.NewHumanMessageFromText("test message")
			}

			ctx := context.Background()
			seq := compiled.Run(ctx, messages, WithMaxInputMessages(tt.maxMessages))

			_, err := Last(seq)
			if tt.expectError {
				require.Error(t, err)
				var validationErr *MessageValidationError
				if assert.ErrorAs(t, err, &validationErr) {
					assert.Equal(t, "message_count", validationErr.Type)
					assert.Equal(t, tt.maxMessages, validationErr.Limit)
					assert.Equal(t, tt.numMessages, validationErr.Actual)
					assert.True(t, errors.Is(validationErr, ErrTooManyMessages))
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMessageValidation_MaxTotalSize(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.
		Node("start", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		}).
		AddEdge(StartNode, "start").
		AddEdge("start", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	tests := []struct {
		name        string
		messages    []message.Message
		maxTotal    int
		expectError bool
	}{
		{
			name: "total within limit",
			messages: []message.Message{
				message.NewHumanMessageFromText(strings.Repeat("A", 300)),
				message.NewHumanMessageFromText(strings.Repeat("B", 300)),
				message.NewHumanMessageFromText(strings.Repeat("C", 300)),
			},
			maxTotal:    1000,
			expectError: false,
		},
		{
			name: "total exceeds limit",
			messages: []message.Message{
				message.NewHumanMessageFromText(strings.Repeat("A", 400)),
				message.NewHumanMessageFromText(strings.Repeat("B", 400)),
				message.NewHumanMessageFromText(strings.Repeat("C", 400)),
			},
			maxTotal:    1000,
			expectError: true,
		},
		{
			name: "no limit",
			messages: []message.Message{
				message.NewHumanMessageFromText(strings.Repeat("X", 10000)),
				message.NewHumanMessageFromText(strings.Repeat("Y", 10000)),
			},
			maxTotal:    0, // unlimited
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			seq := compiled.Run(ctx, tt.messages, WithMaxTotalSize(tt.maxTotal))

			_, err := Last(seq)
			if tt.expectError {
				require.Error(t, err)
				var validationErr *MessageValidationError
				if assert.ErrorAs(t, err, &validationErr) {
					assert.Equal(t, "total_size", validationErr.Type)
					assert.True(t, errors.Is(validationErr, ErrTotalSizeTooLarge))
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMessageValidation_CombinedLimits(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.
		Node("start", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		}).
		AddEdge(StartNode, "start").
		AddEdge("start", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	t.Run("all limits enforced", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText(strings.Repeat("A", 100)),
			message.NewHumanMessageFromText(strings.Repeat("B", 100)),
			message.NewHumanMessageFromText(strings.Repeat("C", 100)),
		}

		ctx := context.Background()
		seq := compiled.Run(ctx, messages,
			WithMaxMessageSize(200), // Each message OK
			WithMaxInputMessages(5), // Count OK
			WithMaxTotalSize(500),   // Total OK (300 bytes)
		)

		_, err := Last(seq)
		require.NoError(t, err)
	})

	t.Run("message count fails first", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText("msg1"),
			message.NewHumanMessageFromText("msg2"),
			message.NewHumanMessageFromText("msg3"),
		}

		ctx := context.Background()
		seq := compiled.Run(ctx, messages,
			WithMaxMessageSize(1000),
			WithMaxInputMessages(2), // Fails here
			WithMaxTotalSize(10000),
		)

		_, err := Last(seq)
		require.Error(t, err)
		var validationErr *MessageValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Equal(t, "message_count", validationErr.Type)
	})

	t.Run("individual message size fails", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText(strings.Repeat("X", 500)),
		}

		ctx := context.Background()
		seq := compiled.Run(ctx, messages,
			WithMaxMessageSize(100), // Fails here
			WithMaxInputMessages(10),
			WithMaxTotalSize(10000),
		)

		_, err := Last(seq)
		require.Error(t, err)
		var validationErr *MessageValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Equal(t, "message_size", validationErr.Type)
		assert.Equal(t, 0, validationErr.MessageIndex)
	})

	t.Run("total size fails", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText(strings.Repeat("A", 80)),
			message.NewHumanMessageFromText(strings.Repeat("B", 80)),
			message.NewHumanMessageFromText(strings.Repeat("C", 80)),
		}

		ctx := context.Background()
		seq := compiled.Run(ctx, messages,
			WithMaxMessageSize(100), // Each message OK
			WithMaxInputMessages(5), // Count OK
			WithMaxTotalSize(200),   // Fails here (total = 240)
		)

		_, err := Last(seq)
		require.Error(t, err)
		var validationErr *MessageValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Equal(t, "total_size", validationErr.Type)
	})
}

func TestMessageValidation_DifferentMessageTypes(t *testing.T) {
	builder, err := NewBuilder()
	require.NoError(t, err)
	builder.
		Node("start", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return &NodeResult{}, nil
		}).
		AddEdge(StartNode, "start").
		AddEdge("start", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	t.Run("function call message", func(t *testing.T) {
		msg := message.NewAIMessage(message.Parts{
			message.FunctionCallPart{
				FunctionCall: &message.FunctionCall{
					ID:        "call_123",
					Name:      "test_function",
					Arguments: `{"arg": "value"}`,
				},
			},
		})

		ctx := context.Background()
		seq := compiled.Run(ctx, []message.Message{msg}, WithMaxMessageSize(100))

		_, err := Last(seq)
		require.NoError(t, err)
	})

	t.Run("multi-part message", func(t *testing.T) {
		msg := message.NewHumanMessage(message.Parts{
			message.TextPart{Text: "Hello"},
			message.DataPart{Data: map[string]any{"key": "value"}},
			message.TextPart{Text: "World"},
		})

		ctx := context.Background()
		seq := compiled.Run(ctx, []message.Message{msg}, WithMaxMessageSize(100))

		_, err := Last(seq)
		require.NoError(t, err)
	})
}

func TestCalculateMessageSize(t *testing.T) {
	tests := []struct {
		name        string
		message     message.Message
		expectedMin int // Minimum expected size
		expectedMax int // Maximum expected size (for approximate calculations)
	}{
		{
			name:        "simple text message",
			message:     message.NewHumanMessageFromText("Hello, World!"),
			expectedMin: 13,
			expectedMax: 13,
		},
		{
			name: "function call message",
			message: message.NewAIMessage(message.Parts{
				message.FunctionCallPart{
					FunctionCall: &message.FunctionCall{
						ID:        "123",
						Name:      "test",
						Arguments: `{"a":"b"}`,
					},
				},
			}),
			expectedMin: 15, // 3 + 4 + 9
			expectedMax: 20,
		},
		{
			name: "multi-part message",
			message: message.NewHumanMessage(message.Parts{
				message.TextPart{Text: "Part1"},
				message.TextPart{Text: "Part2"},
			}),
			expectedMin: 10,
			expectedMax: 10,
		},
		{
			name: "message with data part",
			message: message.NewHumanMessage(message.Parts{
				message.DataPart{Data: map[string]any{
					"key": "value",
				}},
			}),
			expectedMin: 8, // "key" + "value"
			expectedMax: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := calculateMessageSize(tt.message)
			assert.GreaterOrEqual(t, size, tt.expectedMin, "Size should be at least minimum")
			assert.LessOrEqual(t, size, tt.expectedMax, "Size should not exceed maximum")
		})
	}
}
