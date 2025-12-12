// Package integration_test contains integration tests for error recovery behavior.
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

var errTransient = errors.New("transient error")

// TestErrorRecovery_RetrySucceeds tests that retry policy recovers from transient errors.
func TestErrorRecovery_RetrySucceeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var attempts atomic.Int32

	g := graph.New[any, any](resultKey)

	// Node that fails first 2 times, then succeeds
	retryPolicy := &graph.RetryPolicy{
		MaxAttempts: 5,
		Delay:       1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		Multiplier:  2.0,
	}

	g.Node("flaky", graph.WithRetry(func(_ context.Context, _ graph.Scope[any]) (*graph.Command, error) {
		attempt := int(attempts.Add(1))
		if attempt < 3 {
			return nil, errTransient
		}
		return graph.Set(resultKey, "success").End()
	}, retryPolicy), graph.END)

	g.Start("flaky")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(3), attempts.Load())
}

// TestErrorRecovery_RetryExhausted tests that retry fails after max attempts.
func TestErrorRecovery_RetryExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var attempts atomic.Int32

	g := graph.New[any, any](resultKey)

	retryPolicy := &graph.RetryPolicy{
		MaxAttempts: 3,
		Delay:       1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Multiplier:  2.0,
	}

	// Node that always fails
	g.Node("failing", graph.WithRetry(func(_ context.Context, _ graph.Scope[any]) (*graph.Command, error) {
		attempts.Add(1)
		return nil, errTransient
	}, retryPolicy), graph.END)

	g.Start("failing")

	compiled, err := g.Build()
	require.NoError(t, err)

	var foundError error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			foundError = err
		}
	}

	require.Error(t, foundError)
	assert.Equal(t, int32(3), attempts.Load())
}

// TestErrorRecovery_RecoveryMiddleware tests panic recovery in middleware.
func TestErrorRecovery_RecoveryMiddleware(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, any](resultKey)

	// Node that panics
	g.Node("panicky", func(_ context.Context, _ graph.Scope[any]) (*graph.Command, error) {
		panic("intentional panic for testing")
	}, graph.END)

	g.Start("panicky")
	g.WithNodeMiddleware(graphmw.RecoveryMiddleware[any](nil))

	compiled, err := g.Build()
	require.NoError(t, err)

	var foundError error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			foundError = err
		}
	}

	require.Error(t, foundError)
	assert.Contains(t, foundError.Error(), "panic")
}

// TestErrorRecovery_ErrorPropagation tests that errors propagate correctly through the graph.
func TestErrorRecovery_ErrorPropagation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")

	customErr := errors.New("custom application error")

	g := graph.New[any, any](resultKey)

	g.Node("a", func(_ context.Context, _ graph.Scope[any]) (*graph.Command, error) {
		return graph.To("b")
	}, "b")

	g.Node("b", func(_ context.Context, _ graph.Scope[any]) (*graph.Command, error) {
		return nil, customErr
	}, graph.END)

	g.Start("a")

	compiled, err := g.Build()
	require.NoError(t, err)

	var foundError error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			foundError = err
		}
	}

	require.Error(t, foundError)
	// The error message should contain the custom error message
	assert.Contains(t, foundError.Error(), "custom application error")
}

// TestErrorRecovery_ChainedRetry tests retry combined with other wrappers.
func TestErrorRecovery_ChainedRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var attempts atomic.Int32

	g := graph.New[any, any](resultKey)

	retryPolicy := &graph.RetryPolicy{
		MaxAttempts: 3,
		Delay:       1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Multiplier:  2.0,
	}

	// Node wrapped with retry
	g.Node("wrapped", graph.WithRetry(func(_ context.Context, _ graph.Scope[any]) (*graph.Command, error) {
		attempt := int(attempts.Add(1))
		if attempt < 2 {
			return nil, errTransient
		}
		return graph.Set(resultKey, "done").End()
	}, retryPolicy), graph.END)

	g.Start("wrapped")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Should have retried once and then succeeded
	assert.Equal(t, int32(2), attempts.Load())
}
