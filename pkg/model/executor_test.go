package model_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executorMockModel is a test double for model.Model in executor tests
type executorMockModel struct {
	generateFunc func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error]
}

func (m *executorMockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return func(yield func(*model.Response, error) bool) {
		yield(&model.Response{
			Message: message.NewAIMessageFromText("mock response"),
		}, nil)
	}
}

func (m *executorMockModel) Capabilities() model.Capabilities {
	return model.Capabilities{
		Streaming: true,
		Tools:     true,
	}
}


// TestExecutor_BasicExecution tests basic successful execution
func TestExecutor_BasicExecution(t *testing.T) {
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				yield(&model.Response{
					Message: message.NewAIMessageFromText("Hello, world!"),
					Usage: &model.UsageInfo{
						PromptTokens:     10,
						CompletionTokens: 5,
						TotalTokens:      15,
					},
				}, nil)
			}
		},
	}

	executor := model.NewExecutor(mdl, model.WithExecutorName("test-model"))
	require.NotNil(t, executor)

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Hello"),
		},
	}

	resp, err := model.Last(executor.Generate(context.Background(), req))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Hello, world!", message.Stringify(resp.Message))
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

// TestExecutor_StreamingExecution tests streaming response handling
func TestExecutor_StreamingExecution(t *testing.T) {
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				chunks := []string{"Hello", ", ", "world", "!"}
				for i, chunk := range chunks {
					if !yield(&model.Response{
						Message: message.NewAIMessageFromText(chunk),
						Partial: i < len(chunks)-1,
					}, nil) {
						return
					}
				}
			}
		},
	}

	executor := model.NewExecutor(mdl)
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
		Stream:   true,
	}

	var responses []*model.Response
	for resp, err := range executor.Generate(context.Background(), req) {
		require.NoError(t, err)
		responses = append(responses, resp)
	}

	assert.Len(t, responses, 4)
	assert.True(t, responses[0].Partial)
	assert.True(t, responses[1].Partial)
	assert.True(t, responses[2].Partial)
	assert.False(t, responses[3].Partial)

	// Concatenate all chunks
	var result string
	for _, r := range responses {
		result += message.Stringify(r.Message)
	}
	assert.Equal(t, "Hello, world!", result)
}

// TestExecutor_ErrorHandling tests error propagation
func TestExecutor_ErrorHandling(t *testing.T) {
	expectedErr := errors.New("model error")
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				yield(nil, expectedErr)
			}
		},
	}

	executor := model.NewExecutor(mdl, model.WithExecutorName("error-model"))
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	resp, err := model.Last(executor.Generate(context.Background(), req))
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, expectedErr, err)
}

// TestExecutor_NoResponse tests handling when model generates no responses
func TestExecutor_NoResponse(t *testing.T) {
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				// Generate no responses
			}
		},
	}

	executor := model.NewExecutor(mdl)
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	resp, err := model.Last(executor.Generate(context.Background(), req))
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, model.ErrNoResponse, err)
}





// TestExecutor_CancelledContext tests context cancellation handling
func TestExecutor_CancelledContext(t *testing.T) {
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				// Simulate long-running generation
				select {
				case <-ctx.Done():
					yield(nil, ctx.Err())
				default:
					yield(&model.Response{
						Message: message.NewAIMessageFromText("Response"),
					}, nil)
				}
			}
		},
	}

	executor := model.NewExecutor(mdl)
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	resp, err := model.Last(executor.Generate(ctx, req))
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, context.Canceled, err)
}

// TestExecutor_MultipleResponses tests handling multiple response chunks
func TestExecutor_MultipleResponses(t *testing.T) {
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				for i := 0; i < 10; i++ {
					if !yield(&model.Response{
						Message: message.NewAIMessageFromText("chunk"),
						Usage: &model.UsageInfo{
							TotalTokens: i + 1,
						},
					}, nil) {
						return
					}
				}
			}
		},
	}

	executor := model.NewExecutor(mdl)
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	count := 0
	for resp, err := range executor.Generate(context.Background(), req) {
		require.NoError(t, err)
		require.NotNil(t, resp)
		count++
		assert.Equal(t, count, resp.Usage.TotalTokens)
	}

	assert.Equal(t, 10, count)
}

// TestExecutor_ExecutorName tests custom executor name
func TestExecutor_ExecutorName(t *testing.T) {
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				yield(&model.Response{
					Message: message.NewAIMessageFromText("Response"),
				}, nil)
			}
		},
	}

	// Test with custom name
	executor := model.NewExecutor(mdl, model.WithExecutorName("custom-name"))
	require.NotNil(t, executor)

	// Test with default name
	executor2 := model.NewExecutor(mdl)
	require.NotNil(t, executor2)
}

// TestExecutor_StopIteration tests stopping iteration early
func TestExecutor_StopIteration(t *testing.T) {
	chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4", "chunk5"}
	mdl := &executorMockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				for _, chunk := range chunks {
					if !yield(&model.Response{
						Message: message.NewAIMessageFromText(chunk),
					}, nil) {
						return // Consumer stopped iteration
					}
				}
			}
		},
	}

	executor := model.NewExecutor(mdl)
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	count := 0
	for resp, err := range executor.Generate(context.Background(), req) {
		require.NoError(t, err)
		require.NotNil(t, resp)
		count++
		if count == 3 {
			break // Stop after 3 chunks
		}
	}

	assert.Equal(t, 3, count)
}
