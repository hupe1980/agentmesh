package middleware

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
)

// mockLogger captures log calls for testing
type mockLogger struct {
	debugCalls []logCall
	infoCalls  []logCall
	warnCalls  []logCall
	errorCalls []logCall
}

type logCall struct {
	msg  string
	args []any
}

func (m *mockLogger) Debug(msg string, args ...any) {
	m.debugCalls = append(m.debugCalls, logCall{msg, args})
}

func (m *mockLogger) Info(msg string, args ...any) {
	m.infoCalls = append(m.infoCalls, logCall{msg, args})
}

func (m *mockLogger) Warn(msg string, args ...any) {
	m.warnCalls = append(m.warnCalls, logCall{msg, args})
}

func (m *mockLogger) Error(msg string, args ...any) {
	m.errorCalls = append(m.errorCalls, logCall{msg, args})
}

func (m *mockLogger) With(args ...any) logging.Logger {
	return m
}

func (m *mockLogger) reset() {
	m.debugCalls = nil
	m.infoCalls = nil
	m.warnCalls = nil
	m.errorCalls = nil
}

func TestLoggingMiddleware_BasicExecution(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](logger)

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result1", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	// Execute
	ctx := context.Background()
	var outputs []string
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	assert.Equal(t, []string{"result1"}, outputs)

	// Verify log calls
	require.Len(t, logger.infoCalls, 2)
	assert.Equal(t, "Graph execution started", logger.infoCalls[0].msg)
	assert.Equal(t, "Graph execution completed successfully", logger.infoCalls[1].msg)

	// Verify completion log has duration and result count
	args := logger.infoCalls[1].args
	require.Len(t, args, 4) // "duration", value, "results", value
	assert.Equal(t, "duration", args[0])
	assert.IsType(t, time.Duration(0), args[1])
	assert.Equal(t, "results", args[2])
	assert.Equal(t, 1, args[3])
}

func TestLoggingMiddleware_MultipleResults(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, int](logger)

	mockExec := &mockExecutor[string, int]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, int], input string, opts ...graph.RunOption) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				yield(1, nil)
				yield(2, nil)
				yield(3, nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	var outputs []int
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	assert.Equal(t, []int{1, 2, 3}, outputs)

	// Verify result count
	require.Len(t, logger.infoCalls, 2)
	args := logger.infoCalls[1].args
	assert.Equal(t, 3, args[3]) // results count
}

func TestLoggingMiddleware_ExecutionWithError(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](logger)

	expectedErr := errors.New("processing error")
	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result1", nil)
				yield("", expectedErr)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	var outputs []string
	var lastErr error
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		if err != nil {
			lastErr = err
		} else {
			outputs = append(outputs, output)
		}
	}

	assert.Equal(t, []string{"result1"}, outputs)
	assert.Equal(t, expectedErr, lastErr)

	// Verify error was logged
	require.Len(t, logger.errorCalls, 1)
	assert.Equal(t, "Graph execution error", logger.errorCalls[0].msg)
	assert.Equal(t, "processing error", logger.errorCalls[0].args[1])

	// Verify completion warning
	require.Len(t, logger.warnCalls, 1)
	assert.Equal(t, "Graph execution completed with errors", logger.warnCalls[0].msg)
}

func TestLoggingMiddleware_EarlyTermination(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, int](logger)

	mockExec := &mockExecutor[string, int]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, int], input string, opts ...graph.RunOption) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				if !yield(1, nil) {
					return
				}
				if !yield(2, nil) {
					return
				}
				if !yield(3, nil) {
					return
				}
				if !yield(4, nil) {
					return
				}
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	var outputs []int
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
		if len(outputs) == 2 {
			break
		}
	}

	assert.Equal(t, []int{1, 2}, outputs)

	// Verify stopped by consumer log
	require.Len(t, logger.infoCalls, 2)
	assert.Equal(t, "Graph execution stopped by consumer", logger.infoCalls[1].msg)
	args := logger.infoCalls[1].args
	assert.Equal(t, 2, args[3]) // results count
}

func TestLoggingMiddleware_NilLogger(t *testing.T) {
	// Should use logger from context
	middleware := NewLoggingMiddleware[string, string](nil)

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	// Use NoopLogger in context
	ctx := logging.WithLogger(context.Background(), logging.NoopLogger{})

	// Should not panic
	var outputs []string
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	assert.Equal(t, []string{"result"}, outputs)
}

func TestLoggingMiddleware_UsesContextLogger(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](nil) // No logger provided

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	// Put logger in context
	ctx := logging.WithLogger(context.Background(), logger)

	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume results
	}

	// Should have used logger from context
	require.Len(t, logger.infoCalls, 2)
	assert.Equal(t, "Graph execution started", logger.infoCalls[0].msg)
}

func TestLoggingMiddleware_MultipleErrors(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](logger)

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				if !yield("result1", nil) {
					return
				}
				if !yield("", err1) {
					return
				}
				if !yield("result2", nil) {
					return
				}
				if !yield("", err2) {
					return
				}
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume all results
	}

	// Should log both errors
	require.Len(t, logger.errorCalls, 2)
	assert.Contains(t, logger.errorCalls[0].args[1], "error 1")
	assert.Contains(t, logger.errorCalls[1].args[1], "error 2")
}

func TestLoggingMiddleware_DurationTracking(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](logger)

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				time.Sleep(10 * time.Millisecond) // Simulate work
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume results
	}

	require.Len(t, logger.infoCalls, 2)
	args := logger.infoCalls[1].args

	// Check duration is reasonable
	duration := args[1].(time.Duration)
	assert.True(t, duration >= 10*time.Millisecond)
}

func TestLoggingMiddleware_ConsecutiveExecutions(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](logger)

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()

	// Execute twice
	for range wrappedExec.Run(ctx, nil, "input1") {
	}

	logger.reset()

	for range wrappedExec.Run(ctx, nil, "input2") {
	}

	// Second execution should also log properly
	require.Len(t, logger.infoCalls, 2)
	assert.Equal(t, "Graph execution started", logger.infoCalls[0].msg)
	assert.Equal(t, "Graph execution completed successfully", logger.infoCalls[1].msg)
}

func TestLoggingMiddleware_ZeroResults(t *testing.T) {
	logger := &mockLogger{}
	middleware := NewLoggingMiddleware[string, string](logger)

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				// Yield nothing
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	for range wrappedExec.Run(ctx, nil, "input") {
		// No results
	}

	require.Len(t, logger.infoCalls, 2)
	args := logger.infoCalls[1].args
	assert.Equal(t, 0, args[3]) // results count should be 0
}

// Integration test: both middleware together
func TestMiddleware_EventAndLoggingTogether(t *testing.T) {
	logger := &mockLogger{}
	eventMiddleware := NewEventMiddleware[string, string]()
	loggingMiddleware := NewLoggingMiddleware[string, string](logger)

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				if !yield("result1", nil) {
					return
				}
				if !yield("result2", nil) {
					return
				}
			}
		},
	}

	// Apply both middleware (logging wraps event)
	wrappedExec := loggingMiddleware.Wrap(eventMiddleware.Wrap(mockExec))

	ctx := context.Background()
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	var outputs []string
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	// Verify results
	assert.Equal(t, []string{"result1", "result2"}, outputs)

	// Verify logging
	require.Len(t, logger.infoCalls, 2)
	assert.Equal(t, "Graph execution started", logger.infoCalls[0].msg)

	// Verify events
	require.Len(t, capture.events, 2)
	assert.Equal(t, graph.EventGraphStart, capture.events[0].Type)
	assert.Equal(t, graph.EventGraphComplete, capture.events[1].Type)
}

func TestMiddleware_ChainOrder(t *testing.T) {
	var executionOrder []string

	middleware1 := graph.MiddlewareFunc[string, string](func(next graph.Executor[string, string]) graph.Executor[string, string] {
		return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			executionOrder = append(executionOrder, "middleware1-before")
			results := next.Run(ctx, compiled, input, opts...)
			return func(yield func(string, error) bool) {
				executionOrder = append(executionOrder, "middleware1-iter")
				for output, err := range results {
					if !yield(output, err) {
						return
					}
				}
			}
		})
	})

	middleware2 := graph.MiddlewareFunc[string, string](func(next graph.Executor[string, string]) graph.Executor[string, string] {
		return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			executionOrder = append(executionOrder, "middleware2-before")
			results := next.Run(ctx, compiled, input, opts...)
			return func(yield func(string, error) bool) {
				executionOrder = append(executionOrder, "middleware2-iter")
				for output, err := range results {
					if !yield(output, err) {
						return
					}
				}
			}
		})
	})

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				executionOrder = append(executionOrder, "executor")
				yield("result", nil)
			}
		},
	}

	// Chain: middleware1 -> middleware2 -> executor
	wrappedExec := middleware1.Wrap(middleware2.Wrap(mockExec))

	ctx := context.Background()
	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume results
	}

	// Verify execution order
	expected := []string{
		"middleware1-before",
		"middleware2-before",
		"middleware1-iter",
		"middleware2-iter",
		"executor",
	}
	assert.Equal(t, expected, executionOrder)
}

// Benchmark tests
func BenchmarkEventMiddleware(b *testing.B) {
	middleware := NewEventMiddleware[string, string]()

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	eventBus := graph.NewEventBus()
	ctx = graph.WithEventBus(ctx, eventBus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range wrappedExec.Run(ctx, nil, "input") {
			// Consume results
		}
	}
}

func BenchmarkLoggingMiddleware(b *testing.B) {
	middleware := NewLoggingMiddleware[string, string](logging.NoopLogger{})

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range wrappedExec.Run(ctx, nil, "input") {
			// Consume results
		}
	}
}

func BenchmarkBothMiddleware(b *testing.B) {
	eventMiddleware := NewEventMiddleware[string, string]()
	loggingMiddleware := NewLoggingMiddleware[string, string](logging.NoopLogger{})

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := loggingMiddleware.Wrap(eventMiddleware.Wrap(mockExec))

	ctx := context.Background()
	eventBus := graph.NewEventBus()
	ctx = graph.WithEventBus(ctx, eventBus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range wrappedExec.Run(ctx, nil, "input") {
			// Consume results
		}
	}
}
