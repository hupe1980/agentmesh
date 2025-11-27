package graph_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor is a simple test executor that collects execution data
type mockExecutor struct {
	calls []string
}

func (m *mockExecutor) Run(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
	m.calls = append(m.calls, "executor")
	return func(yield func(state.Updates, error) bool) {
		yield(state.Updates{"executed": true}, nil)
	}
}

// trackingMiddleware tracks middleware execution order
type trackingMiddleware struct {
	name  string
	calls *[]string
}

func (tm *trackingMiddleware) Wrap(next graph.Executor[[]message.Message, state.Updates]) graph.Executor[[]message.Message, state.Updates] {
	return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
		*tm.calls = append(*tm.calls, tm.name+"-before")
		results := next.Run(ctx, compiled, input, opts...)
		// Wrap the iterator to track "after"
		return func(yield func(state.Updates, error) bool) {
			for update, err := range results {
				if !yield(update, err) {
					break
				}
			}
			*tm.calls = append(*tm.calls, tm.name+"-after")
		}
	})
}

func TestMiddlewareFunc_Wrap(t *testing.T) {
	var calls []string

	middlewareFunc := graph.MiddlewareFunc[[]message.Message, state.Updates](func(next graph.Executor[[]message.Message, state.Updates]) graph.Executor[[]message.Message, state.Updates] {
		return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
			calls = append(calls, "middleware")
			return next.Run(ctx, compiled, input, opts...)
		})
	})

	executor := &mockExecutor{calls: []string{}}
	wrapped := middlewareFunc.Wrap(executor)

	// Execute wrapped middleware
	ctx := context.Background()
	results := wrapped.Run(ctx, nil, nil)

	// Consume the iterator
	for _, err := range results {
		require.NoError(t, err)
	}

	assert.Contains(t, calls, "middleware")
	assert.Contains(t, executor.calls, "executor")
}

func TestChain_SingleMiddleware(t *testing.T) {
	var calls []string

	executor := &mockExecutor{calls: []string{}}
	middleware := &trackingMiddleware{
		name:  "mw1",
		calls: &calls,
	}

	chained := graph.Chain(executor, middleware)

	ctx := context.Background()
	results := chained.Run(ctx, nil, nil)

	// Consume iterator
	for _, err := range results {
		require.NoError(t, err)
	}

	// Check execution order
	require.Len(t, calls, 2)
	assert.Equal(t, "mw1-before", calls[0])
	assert.Equal(t, "mw1-after", calls[1])
	assert.Contains(t, executor.calls, "executor")
}

func TestChain_MultipleMiddleware(t *testing.T) {
	var calls []string

	executor := &mockExecutor{calls: []string{}}
	mw1 := &trackingMiddleware{name: "mw1", calls: &calls}
	mw2 := &trackingMiddleware{name: "mw2", calls: &calls}
	mw3 := &trackingMiddleware{name: "mw3", calls: &calls}

	// Chain middleware - first should be outermost
	chained := graph.Chain(executor, mw1, mw2, mw3)

	ctx := context.Background()
	results := chained.Run(ctx, nil, nil)

	// Consume iterator
	for _, err := range results {
		require.NoError(t, err)
	}

	// Verify order: mw1 → mw2 → mw3 → executor → mw3 → mw2 → mw1
	require.Len(t, calls, 6)
	assert.Equal(t, "mw1-before", calls[0])
	assert.Equal(t, "mw2-before", calls[1])
	assert.Equal(t, "mw3-before", calls[2])
	assert.Equal(t, "mw3-after", calls[3])
	assert.Equal(t, "mw2-after", calls[4])
	assert.Equal(t, "mw1-after", calls[5])
}

func TestChain_NoMiddleware(t *testing.T) {
	executor := &mockExecutor{calls: []string{}}

	// Chain with no middleware should return original executor
	chained := graph.Chain(executor)

	ctx := context.Background()
	results := chained.Run(ctx, nil, nil)

	// Consume iterator
	for _, err := range results {
		require.NoError(t, err)
	}

	assert.Contains(t, executor.calls, "executor")
}

func TestWrapFunc(t *testing.T) {
	t.Run("simple_wrapper", func(t *testing.T) {
		var executed bool

		wrapper := graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
			executed = true
			return func(yield func(state.Updates, error) bool) {
				yield(state.Updates{"result": "success"}, nil)
			}
		})

		results := wrapper.Run(context.Background(), nil, nil)

		var updates []state.Updates
		for update, err := range results {
			require.NoError(t, err)
			updates = append(updates, update)
		}

		assert.True(t, executed)
		require.Len(t, updates, 1)
		assert.Equal(t, "success", updates[0]["result"])
	})

	t.Run("error_handling", func(t *testing.T) {
		expectedErr := errors.New("execution failed")

		wrapper := graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
			return func(yield func(state.Updates, error) bool) {
				yield(nil, expectedErr)
			}
		})

		results := wrapper.Run(context.Background(), nil, nil)

		for _, err := range results {
			assert.Equal(t, expectedErr, err)
		}
	})

	t.Run("multiple_yields", func(t *testing.T) {
		wrapper := graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
			return func(yield func(state.Updates, error) bool) {
				if !yield(state.Updates{"step": 1}, nil) {
					return
				}
				if !yield(state.Updates{"step": 2}, nil) {
					return
				}
				yield(state.Updates{"step": 3}, nil)
			}
		})

		results := wrapper.Run(context.Background(), nil, nil)

		var steps []int
		for update, err := range results {
			require.NoError(t, err)
			steps = append(steps, update["step"].(int))
		}

		assert.Equal(t, []int{1, 2, 3}, steps)
	})
}

func TestExecutorWrapper_Run(t *testing.T) {
	var capturedCtx context.Context
	var capturedInput []message.Message

	wrapper := graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
		capturedCtx = ctx
		capturedInput = input
		return func(yield func(state.Updates, error) bool) {
			yield(state.Updates{}, nil)
		}
	})

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("test")}

	results := wrapper.Run(ctx, nil, input)

	// Consume iterator
	for _, err := range results {
		require.NoError(t, err)
	}

	assert.Equal(t, ctx, capturedCtx)
	assert.Equal(t, input, capturedInput)
}

func TestMiddleware_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("middleware error")

	errorMiddleware := graph.MiddlewareFunc[[]message.Message, state.Updates](func(next graph.Executor[[]message.Message, state.Updates]) graph.Executor[[]message.Message, state.Updates] {
		return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
			return func(yield func(state.Updates, error) bool) {
				yield(nil, expectedErr)
			}
		})
	})

	executor := &mockExecutor{calls: []string{}}
	wrapped := errorMiddleware.Wrap(executor)

	results := wrapped.Run(context.Background(), nil, nil)

	for _, err := range results {
		assert.Equal(t, expectedErr, err)
	}

	// Executor should not have been called due to error
	assert.Empty(t, executor.calls)
}

func TestMiddleware_ContextPropagation(t *testing.T) {
	type ctxKey string
	const testKey ctxKey = "test"

	middleware := graph.MiddlewareFunc[[]message.Message, state.Updates](func(next graph.Executor[[]message.Message, state.Updates]) graph.Executor[[]message.Message, state.Updates] {
		return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
			// Add value to context
			ctx = context.WithValue(ctx, testKey, "middleware-value")
			return next.Run(ctx, compiled, input, opts...)
		})
	})

	var capturedValue string
	testExecutor := graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[[]message.Message, state.Updates], input []message.Message, opts ...graph.RunOption) iter.Seq2[state.Updates, error] {
		if val := ctx.Value(testKey); val != nil {
			capturedValue = val.(string)
		}
		return func(yield func(state.Updates, error) bool) {
			yield(state.Updates{}, nil)
		}
	})

	wrapped := middleware.Wrap(testExecutor)
	results := wrapped.Run(context.Background(), nil, nil)

	// Consume iterator
	for _, err := range results {
		require.NoError(t, err)
	}

	assert.Equal(t, "middleware-value", capturedValue)
}
