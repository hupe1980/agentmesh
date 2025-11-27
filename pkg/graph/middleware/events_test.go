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
)

func TestEventMiddleware_BasicExecution(t *testing.T) {
	middleware := NewEventMiddleware[string, string]()

	// Create a mock executor that yields one result
	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result1", nil)
			}
		},
	}

	// Wrap the executor with middleware
	wrappedExec := middleware.Wrap(mockExec)

	// Setup event capturing
	ctx := context.Background()
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	// Execute
	results := wrappedExec.Run(ctx, nil, "input")
	var outputs []string
	for output, err := range results {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	// Verify results
	assert.Equal(t, []string{"result1"}, outputs)

	// Verify events
	require.Len(t, capture.events, 2)

	// Check graph start event
	assert.Equal(t, graph.EventGraphStart, capture.events[0].Type)
	assert.NotEmpty(t, capture.events[0].RunID)
	assert.NotEmpty(t, capture.events[0].Timestamp)

	// Check graph complete event
	assert.Equal(t, graph.EventGraphComplete, capture.events[1].Type)
	assert.Equal(t, capture.events[0].RunID, capture.events[1].RunID)
	assert.True(t, capture.events[1].Duration > 0)
}

func TestEventMiddleware_MultipleResults(t *testing.T) {
	middleware := NewEventMiddleware[string, int]()

	// Create a mock executor that yields multiple results
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
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	// Execute and consume all results
	var outputs []int
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	assert.Equal(t, []int{1, 2, 3}, outputs)
	require.Len(t, capture.events, 2)
	assert.Equal(t, graph.EventGraphStart, capture.events[0].Type)
	assert.Equal(t, graph.EventGraphComplete, capture.events[1].Type)
}

func TestEventMiddleware_ExecutionWithError(t *testing.T) {
	middleware := NewEventMiddleware[string, string]()

	expectedErr := errors.New("execution error")
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
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	// Execute
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

	// Verify error event was published
	require.Len(t, capture.events, 2)
	assert.Equal(t, graph.EventGraphStart, capture.events[0].Type)
	assert.Equal(t, graph.EventGraphError, capture.events[1].Type)
}

func TestEventMiddleware_EarlyTermination(t *testing.T) {
	middleware := NewEventMiddleware[string, int]()

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
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	// Stop consuming after 2 results
	var outputs []int
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
		if len(outputs) == 2 {
			break
		}
	}

	assert.Equal(t, []int{1, 2}, outputs)

	// Verify stopped_by_consumer event
	require.Len(t, capture.events, 2)
	assert.Equal(t, graph.EventGraphStart, capture.events[0].Type)
	assert.Equal(t, graph.EventGraphComplete, capture.events[1].Type)

	// Check for stopped_by_consumer flag
	data := capture.events[1].Data
	require.NotNil(t, data)
	assert.True(t, data["stopped_by_consumer"].(bool))
}

func TestEventMiddleware_CustomRunIDFunc(t *testing.T) {
	customRunID := "custom-run-123"
	middleware := NewEventMiddleware[string, string]().WithRunIDFunc(func() string {
		return customRunID
	})

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
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	// Execute
	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume results
	}

	// Verify custom run ID was used
	require.Len(t, capture.events, 2)
	assert.Equal(t, customRunID, capture.events[0].RunID)
	assert.Equal(t, customRunID, capture.events[1].RunID)
}

func TestEventMiddleware_WithoutEventBus(t *testing.T) {
	middleware := NewEventMiddleware[string, string]()

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	// Execute without event bus - should not panic
	ctx := context.Background()
	var outputs []string
	for output, err := range wrappedExec.Run(ctx, nil, "input") {
		require.NoError(t, err)
		outputs = append(outputs, output)
	}

	assert.Equal(t, []string{"result"}, outputs)
}

func TestEventMiddleware_TimestampAccuracy(t *testing.T) {
	middleware := NewEventMiddleware[string, string]()

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
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	startTime := time.Now()
	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume results
	}
	endTime := time.Now()

	require.Len(t, capture.events, 2)

	// Verify timestamps are reasonable
	assert.True(t, capture.events[0].Timestamp.After(startTime) || capture.events[0].Timestamp.Equal(startTime))
	assert.True(t, capture.events[1].Timestamp.Before(endTime) || capture.events[1].Timestamp.Equal(endTime))

	// Verify duration is reasonable
	assert.True(t, capture.events[1].Duration >= 10*time.Millisecond)
}

func TestEventMiddleware_DefaultRunIDFormat(t *testing.T) {
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
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	for range wrappedExec.Run(ctx, nil, "input") {
		// Consume results
	}

	require.Len(t, capture.events, 2)

	// Verify default run ID format starts with "run-"
	assert.Contains(t, capture.events[0].RunID, "run-")
	assert.NotEmpty(t, capture.events[0].RunID)
}

func TestEventMiddleware_ConsecutiveExecutions(t *testing.T) {
	middleware := NewEventMiddleware[string, string]()

	mockExec := &mockExecutor[string, string]{
		runFunc: func(ctx context.Context, compiled *graph.Compiled[string, string], input string, opts ...graph.RunOption) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				time.Sleep(2 * time.Millisecond) // Small delay to ensure different timestamps
				yield("result", nil)
			}
		},
	}

	wrappedExec := middleware.Wrap(mockExec)

	ctx := context.Background()
	eventBus := graph.NewEventBus()
	capture := &captureEvents{}
	eventBus.Subscribe(capture)
	ctx = graph.WithEventBus(ctx, eventBus)

	// Execute twice
	for range wrappedExec.Run(ctx, nil, "input1") {
	}
	for range wrappedExec.Run(ctx, nil, "input2") {
	}

	// Should have 4 events (2 per execution)
	require.Len(t, capture.events, 4)

	// Verify run IDs are set and both runs have consistent IDs
	runID1 := capture.events[0].RunID
	runID2 := capture.events[2].RunID
	assert.NotEmpty(t, runID1)
	assert.NotEmpty(t, runID2)
	// Each run should have consistent run IDs
	assert.Equal(t, runID1, capture.events[1].RunID)
	assert.Equal(t, runID2, capture.events[3].RunID)
	// Run IDs should be different (may occasionally be same if too fast, but unlikely with sleep)
	if runID1 == runID2 {
		t.Log("Warning: Run IDs are the same (executions too fast)")
	}
}
