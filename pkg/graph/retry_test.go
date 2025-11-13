package graph

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ErrTransient = errors.New("transient error")
	ErrPermanent = errors.New("permanent error")
)

func TestRetryPolicy(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "success",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return &NodeResult{Updates: map[string]any{"result": "ok"}}, nil
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "success")
		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)

		assert.Equal(t, 1, attempts, "should succeed on first attempt")

		result, ok := compiled.State().Get("result").(string)
		require.True(t, ok, "result should be a string")
		assert.Equal(t, "ok", result)
	})

	t.Run("retries and eventually succeeds", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "retry-succeed",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				if attempts < 3 {
					return nil, fmt.Errorf("attempt %d failed: %w", attempts, ErrTransient)
				}
				return &NodeResult{Updates: map[string]any{"attempts": attempts}}, nil
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 5,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "retry-succeed")
		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)

		assert.Equal(t, 3, attempts)

		actualAttempts, ok := compiled.State().Get("attempts").(int)
		require.True(t, ok, "attempts should be an int")
		assert.Equal(t, 3, actualAttempts)
	})

	t.Run("exhausts retries and fails", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "always-fail",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrTransient
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "always-fail")
		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.Error(t, err, "should fail after exhausting retries")

		assert.Equal(t, 3, attempts)
		assert.NotEmpty(t, err.Error())
	})

	t.Run("custom retryable function skips non-retryable errors", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "permanent-fail",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrPermanent
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 5,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
				Retryable: func(err error) bool {
					return errors.Is(err, ErrTransient)
				},
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "permanent-fail")
		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.Error(t, err)

		// Should only attempt once since error is not retryable
		assert.Equal(t, 1, attempts, "non-retryable error should not retry")
	})

	t.Run("respects context cancellation during backoff", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "cancel-during-retry",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrTransient
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 10,
				Backoff:     func(n int) time.Duration { return 5 * time.Second }, // Long backoff
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "cancel-during-retry")
		compiled, err := g.Compile()
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		_, err = Last(compiled.Run(ctx, nil))
		elapsed := time.Since(start)

		// Error can be nil if cancellation happens between graph execution steps
		// or non-nil if caught during retry - both are acceptable
		_ = err

		// Should exit quickly (within 1 second), not wait for full backoff
		assert.Less(t, elapsed, time.Second, "should cancel quickly")

		// Should have attempted at least once
		assert.GreaterOrEqual(t, attempts, 1, "should attempt at least once")
	})

	t.Run("no retry policy executes once", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "no-retry",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrTransient
			},
			// No RetryPolicy set
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "no-retry")
		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.Error(t, err)

		assert.Equal(t, 1, attempts, "should execute exactly once without retry policy")
	})

	t.Run("exponential backoff timing", func(t *testing.T) {
		tests := []struct {
			attempt  int
			expected time.Duration
		}{
			{0, 0},
			{1, 1 * time.Second},
			{2, 2 * time.Second},
			{3, 4 * time.Second},
			{4, 8 * time.Second},
		}

		for _, tt := range tests {
			t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
				result := DefaultBackoff(tt.attempt)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("aggregates reset between retry attempts", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "retry-aggregate",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				_ = s.Aggregate("total", 1)
				if attempts == 1 {
					return nil, ErrTransient
				}
				_ = s.Aggregate("total", 2)
				return nil, nil
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				Backoff:     func(int) time.Duration { return time.Millisecond },
			},
		})
		require.NoError(t, err)

		err = g.AddNode(&Node{
			Name: "report",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				snap := s.AggregatesSnapshot()
				var total float64
				if snap != nil {
					switch v := snap["total"].(type) {
					case float64:
						total = v
					case int:
						total = float64(v)
					}
				}
				return &NodeResult{Updates: map[string]any{"observed": total}}, nil
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "retry-aggregate")
		g.AddEdge("retry-aggregate", "report")
		g.AddEdge("report", EndNode)

		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil, WithAggregators(map[string]pregel.Aggregator{"total": &SumAggregator{}})))
		require.NoError(t, err)

		assert.Equal(t, 2, attempts)

		observedValue := compiled.State().Get("observed")
		value, ok := observedValue.(float64)
		if !ok {
			if intValue, ok := observedValue.(int); ok {
				value = float64(intValue)
			} else {
				t.Fatalf("unexpected observed value type %T", observedValue)
			}
		}
		assert.Equal(t, 3.0, value, "aggregates should be reset between retries (1+2=3, not 1+1+2=4)")
	})

	t.Run("preserves all retry attempt errors", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		attempts := 0
		err = g.AddNode(&Node{
			Name: "failing-node",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, fmt.Errorf("error from attempt %d", attempts)
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "failing-node")
		g.AddEdge("failing-node", EndNode)

		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.Error(t, err, "should fail after exhausting retries")

		// Check for RetryExhaustedError with all attempts preserved
		var retryErr *RetryExhaustedError
		require.ErrorAs(t, err, &retryErr, "should be RetryExhaustedError")

		assert.Len(t, retryErr.Attempts, 3, "should preserve all 3 attempts")
		assert.Equal(t, "failing-node", retryErr.Node)

		// Verify each attempt error is present
		for i, attemptErr := range retryErr.Attempts {
			require.NotNil(t, attemptErr, "attempt %d error should not be nil", i+1)
			assert.Contains(t, attemptErr.Error(), fmt.Sprintf("attempt %d", i+1))
			assert.Contains(t, attemptErr.Error(), fmt.Sprintf("error from attempt %d", i+1))
		}

		// Verify the actual number of attempts made
		assert.Equal(t, 3, attempts)
	})
}
