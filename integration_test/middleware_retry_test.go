package integration_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMiddleware_ChainedExecution tests that middleware executes in correct order
func TestMiddleware_ChainedExecution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var order []string

	// Create middleware that records execution order
	recordMiddleware := func(name string) graph.Middleware[any] {
		return func(next graph.NodeFunc[any]) graph.NodeFunc[any] {
			return func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
				order = append(order, name+"-before")
				cmd, err := next(ctx, scope)
				order = append(order, name+"-after")
				return cmd, err
			}
		}
	}

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, any](resultKey)
	g.Node("process", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		order = append(order, "node")
		return graph.Set(resultKey, "done").End()
	}, graph.END)
	g.Start("process")
	g.WithMiddleware(graph.Chain(
		recordMiddleware("mw1"),
		recordMiddleware("mw2"),
		recordMiddleware("mw3"),
	))

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Verify order: mw1 -> mw2 -> mw3 -> node -> mw3 -> mw2 -> mw1
	expected := []string{
		"mw1-before", "mw2-before", "mw3-before",
		"node",
		"mw3-after", "mw2-after", "mw1-after",
	}
	assert.Equal(t, expected, order)
}

// TestMiddleware_TimingTracking tests timing middleware accurately tracks duration
func TestMiddleware_TimingTracking(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var recordedDurations []time.Duration

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, any](resultKey)
	g.Node("slow", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		time.Sleep(50 * time.Millisecond)
		return graph.Set(resultKey, "done").End()
	}, graph.END)
	g.Start("slow")
	g.WithMiddleware(graphmw.TimingMiddleware[any](func(nodeName string, d time.Duration) {
		recordedDurations = append(recordedDurations, d)
	}))

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	require.Len(t, recordedDurations, 1)
	assert.GreaterOrEqual(t, recordedDurations[0], 50*time.Millisecond)
}

// TestMiddleware_RecoveryFromPanic tests that recovery middleware catches panics
func TestMiddleware_RecoveryFromPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var recoveredPanic any

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, any](resultKey)
	g.Node("panicker", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		panic("intentional panic for testing")
	}, graph.END)
	g.Start("panicker")
	g.WithMiddleware(graphmw.RecoveryMiddleware[any](func(nodeName string, recovered any) {
		recoveredPanic = recovered
	}))

	compiled, err := g.Build()
	require.NoError(t, err)

	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	assert.NotNil(t, lastErr)
	assert.Equal(t, "intentional panic for testing", recoveredPanic)
}

// TestRetry_SuccessAfterFailures tests retry succeeds after transient failures
func TestRetry_SuccessAfterFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var attempts int32

	resultKey := graph.NewKey[string]("result")

	retryNode := graph.WithRetry(func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return nil, errors.New("transient failure")
		}
		return graph.Set(resultKey, "success").End()
	}, &graph.RetryPolicy{
		MaxAttempts: 5,
		Delay:       10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Multiplier:  2.0,
	})

	g := graph.New[any, any](resultKey)
	g.Node("retry", retryNode, graph.END)
	g.Start("retry")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

// TestRetry_ExceedsMaxAttempts tests retry gives up after max attempts
func TestRetry_ExceedsMaxAttempts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var attempts int32

	resultKey := graph.NewKey[string]("result")

	retryNode := graph.WithRetry(func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("persistent failure")
	}, &graph.RetryPolicy{
		MaxAttempts: 3,
		Delay:       5 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Multiplier:  2.0,
	})

	g := graph.New[any, any](resultKey)
	g.Node("retry", retryNode, graph.END)
	g.Start("retry")

	compiled, err := g.Build()
	require.NoError(t, err)

	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	assert.Error(t, lastErr)
	assert.Contains(t, lastErr.Error(), "max retries")
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

// TestRetry_NonRetryableError tests retry respects non-retryable errors
func TestRetry_NonRetryableError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var attempts int32

	permanentErr := errors.New("permanent error")

	resultKey := graph.NewKey[string]("result")

	retryNode := graph.WithRetry(func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, permanentErr
	}, &graph.RetryPolicy{
		MaxAttempts: 5,
		Delay:       5 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Multiplier:  2.0,
		Retryable: func(err error) bool {
			return !errors.Is(err, permanentErr)
		},
	})

	g := graph.New[any, any](resultKey)
	g.Node("retry", retryNode, graph.END)
	g.Start("retry")

	compiled, err := g.Build()
	require.NoError(t, err)

	var lastErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			lastErr = err
		}
	}

	assert.ErrorIs(t, lastErr, permanentErr)
	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts)) // Only 1 attempt, no retries
}

// TestRetry_ContextCancellation tests retry respects context cancellation
func TestRetry_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var attempts int32

	resultKey := graph.NewKey[string]("result")

	retryNode := graph.WithRetry(func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("keep failing")
	}, &graph.RetryPolicy{
		MaxAttempts: 100,
		Delay:       50 * time.Millisecond,
		MaxDelay:    500 * time.Millisecond,
		Multiplier:  2.0,
	})

	g := graph.New[any, any](resultKey)
	g.Node("retry", retryNode, graph.END)
	g.Start("retry")

	compiled, err := g.Build()
	require.NoError(t, err)

	for range compiled.Run(ctx, nil) {
		// consume results
	}

	// Should have been cancelled before reaching max attempts
	assert.Less(t, atomic.LoadInt32(&attempts), int32(100))
}
