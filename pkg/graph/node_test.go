package graph_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := graph.DefaultRetryPolicy()

	assert.Equal(t, 3, policy.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, policy.Delay)
	assert.Equal(t, 5*time.Second, policy.MaxDelay)
	assert.Equal(t, 2.0, policy.Multiplier)
	assert.Nil(t, policy.Retryable)
}

func TestRetryPolicyBuilder(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		policy := graph.NewRetryPolicyBuilder().Build()

		assert.Equal(t, 3, policy.MaxAttempts)
		assert.Equal(t, 100*time.Millisecond, policy.Delay)
		assert.Equal(t, 5*time.Second, policy.MaxDelay)
		assert.Equal(t, 2.0, policy.Multiplier)
	})

	t.Run("with max attempts", func(t *testing.T) {
		policy := graph.NewRetryPolicyBuilder().
			WithMaxAttempts(5).
			Build()

		assert.Equal(t, 5, policy.MaxAttempts)
	})

	t.Run("with exponential backoff", func(t *testing.T) {
		policy := graph.NewRetryPolicyBuilder().
			WithExponentialBackoff(time.Second, 3.0).
			Build()

		assert.Equal(t, time.Second, policy.Delay)
		assert.Equal(t, 3.0, policy.Multiplier)
	})

	t.Run("with linear backoff", func(t *testing.T) {
		policy := graph.NewRetryPolicyBuilder().
			WithLinearBackoff(500 * time.Millisecond).
			Build()

		assert.Equal(t, 500*time.Millisecond, policy.Delay)
		assert.Equal(t, 1.0, policy.Multiplier)
	})

	t.Run("with constant backoff", func(t *testing.T) {
		policy := graph.NewRetryPolicyBuilder().
			WithConstantBackoff(2 * time.Second).
			Build()

		assert.Equal(t, 2*time.Second, policy.Delay)
		assert.Equal(t, 1.0, policy.Multiplier)
		assert.Equal(t, 2*time.Second, policy.MaxDelay)
	})

	t.Run("with max delay", func(t *testing.T) {
		policy := graph.NewRetryPolicyBuilder().
			WithMaxDelay(10 * time.Second).
			Build()

		assert.Equal(t, 10*time.Second, policy.MaxDelay)
	})

	t.Run("with retryable func", func(t *testing.T) {
		retryable := func(err error) bool {
			return errors.Is(err, context.DeadlineExceeded)
		}
		policy := graph.NewRetryPolicyBuilder().
			WithRetryableFunc(retryable).
			Build()

		assert.NotNil(t, policy.Retryable)
	})
}

func TestWithRetry(t *testing.T) {
	t.Run("no retry on success", func(t *testing.T) {
		attempts := 0
		fn := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
			attempts++
			return graph.To(graph.END)
		}

		wrapped := graph.WithRetry(fn, graph.DefaultRetryPolicy())
		_, err := wrapped(context.Background(), nil)

		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("retry on failure", func(t *testing.T) {
		attempts := 0
		fn := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("transient error")
			}
			return graph.To(graph.END)
		}

		policy := &graph.RetryPolicy{
			MaxAttempts: 5,
			Delay:       1 * time.Millisecond,
			MaxDelay:    10 * time.Millisecond,
			Multiplier:  2.0,
		}

		wrapped := graph.WithRetry(fn, policy)
		_, err := wrapped(context.Background(), nil)

		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		attempts := 0
		fn := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
			attempts++
			return nil, errors.New("persistent error")
		}

		policy := &graph.RetryPolicy{
			MaxAttempts: 3,
			Delay:       1 * time.Millisecond,
			MaxDelay:    10 * time.Millisecond,
			Multiplier:  2.0,
		}

		wrapped := graph.WithRetry(fn, policy)
		_, err := wrapped(context.Background(), nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max retries (3) exceeded")
		assert.Equal(t, 3, attempts)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		attempts := 0
		permanentErr := errors.New("permanent error")
		fn := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
			attempts++
			return nil, permanentErr
		}

		policy := &graph.RetryPolicy{
			MaxAttempts: 5,
			Delay:       1 * time.Millisecond,
			MaxDelay:    10 * time.Millisecond,
			Multiplier:  2.0,
			Retryable:   func(err error) bool { return !errors.Is(err, permanentErr) },
		}

		wrapped := graph.WithRetry(fn, policy)
		_, err := wrapped(context.Background(), nil)

		assert.ErrorIs(t, err, permanentErr)
		assert.Equal(t, 1, attempts) // No retries
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0

		fn := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
			attempts++
			if attempts == 1 {
				cancel() // Cancel after first attempt
			}
			return nil, errors.New("error")
		}

		policy := &graph.RetryPolicy{
			MaxAttempts: 5,
			Delay:       100 * time.Millisecond, // Long delay to ensure we hit cancellation
			MaxDelay:    1 * time.Second,
			Multiplier:  2.0,
		}

		wrapped := graph.WithRetry(fn, policy)
		_, err := wrapped(ctx, nil)

		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("nil policy passes through", func(t *testing.T) {
		called := false
		fn := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
			called = true
			return graph.To(graph.END)
		}

		wrapped := graph.WithRetry(fn, nil)
		_, err := wrapped(context.Background(), nil)

		require.NoError(t, err)
		assert.True(t, called)
	})
}

func TestNamespace(t *testing.T) {
	t.Run("create and use", func(t *testing.T) {
		ns := graph.NewNamespace("agent1")

		assert.Equal(t, "agent1", ns.Name())
		assert.Equal(t, "agent1.counter", ns.Prefix("counter"))
	})

	t.Run("prefix with nested key", func(t *testing.T) {
		ns := graph.NewNamespace("module")
		assert.Equal(t, "module.sub.key", ns.Prefix("sub.key"))
	})
}

func TestWithRetryInGraph(t *testing.T) {
	attempts := 0

	retryingNode := graph.WithRetry(func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("transient")
		}
		return graph.To(graph.END)
	}, &graph.RetryPolicy{
		MaxAttempts: 3,
		Delay:       1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Multiplier:  2.0,
	})

	counterKey := graph.NewKey[int]("counter")
	g := graph.New(counterKey)
	g.Node("retry", retryingNode, graph.END)
	g.Start("retry")

	compiled, err := g.Build()
	require.NoError(t, err)

	for range compiled.Run(context.Background(), nil) {
	}

	assert.Equal(t, 2, attempts)
}
