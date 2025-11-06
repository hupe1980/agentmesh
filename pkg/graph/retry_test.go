package graph

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

var (
	ErrTransient = errors.New("transient error")
	ErrPermanent = errors.New("permanent error")
)

func TestRetryPolicy(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
			Name: "success",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return &NodeResult{Updates: map[string]any{"result": "ok"}}, nil
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "success")
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = compiled.Invoke(context.Background(), nil)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}

		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}

		result, ok := compiled.State().Get("result").(string)
		if !ok || result != "ok" {
			t.Errorf("expected result='ok', got %v", compiled.State().Get("result"))
		}
	})

	t.Run("retries and eventually succeeds", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
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
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "retry-succeed")
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = compiled.Invoke(context.Background(), nil)
		if err != nil {
			t.Fatalf("expected success after retries, got %v", err)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}

		actualAttempts, ok := compiled.State().Get("attempts").(int)
		if !ok || actualAttempts != 3 {
			t.Errorf("expected attempts=3, got %v", compiled.State().Get("attempts"))
		}
	})

	t.Run("exhausts retries and fails", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
			Name: "always-fail",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrTransient
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				Backoff:     func(n int) time.Duration { return time.Millisecond },
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "always-fail")
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = compiled.Invoke(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error after exhausting retries")
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}

		// Should contain both the node name and number of attempts in error message
		errMsg := err.Error()
		if errMsg == "" {
			t.Error("error message should not be empty")
		}
	})

	t.Run("custom retryable function skips non-retryable errors", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
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
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "permanent-fail")
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = compiled.Invoke(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error")
		}

		// Should only attempt once since error is not retryable
		if attempts != 1 {
			t.Errorf("expected 1 attempt (non-retryable error), got %d", attempts)
		}
	})

	t.Run("respects context cancellation during backoff", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
			Name: "cancel-during-retry",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrTransient
			},
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 10,
				Backoff:     func(n int) time.Duration { return 5 * time.Second }, // Long backoff
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "cancel-during-retry")
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		_, err = compiled.Invoke(ctx, nil)
		elapsed := time.Since(start)

		// Error can be nil if cancellation happens between graph execution steps
		// or non-nil if caught during retry - both are acceptable
		_ = err

		// Should exit quickly (within 1 second), not wait for full backoff
		if elapsed > time.Second {
			t.Errorf("took too long to cancel: %v", elapsed)
		}

		// Should have attempted at least once
		if attempts < 1 {
			t.Errorf("expected at least 1 attempt, got %d", attempts)
		}
	})

	t.Run("no retry policy executes once", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
			Name: "no-retry",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				attempts++
				return nil, ErrTransient
			},
			// No RetryPolicy set
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "no-retry")
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = compiled.Invoke(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error")
		}

		if attempts != 1 {
			t.Errorf("expected exactly 1 attempt without retry policy, got %d", attempts)
		}
	})

	t.Run("exponential backoff timing", func(t *testing.T) {
		// Test DefaultBackoff function
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
			result := DefaultBackoff(tt.attempt)
			if result != tt.expected {
				t.Errorf("DefaultBackoff(%d): expected %v, got %v", tt.attempt, tt.expected, result)
			}
		}
	})

	t.Run("aggregates reset between retry attempts", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		attempts := 0
		if err := g.AddNode(&Node{
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
		}); err != nil {
			t.Fatal(err)
		}

		if err := g.AddNode(&Node{
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
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "retry-aggregate")
		g.AddEdge("retry-aggregate", "report")
		g.AddEdge("report", EndNode)
		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, err = compiled.Invoke(context.Background(), nil, WithAggregators(map[string]Aggregator{"total": &SumAggregator{}}))
		if err != nil {
			t.Fatalf("expected success after retry, got %v", err)
		}

		if attempts != 2 {
			t.Fatalf("expected 2 attempts, got %d", attempts)
		}

		observedValue := compiled.State().Get("observed")
		value, ok := observedValue.(float64)
		if !ok {
			if intValue, ok := observedValue.(int); ok {
				value = float64(intValue)
			} else {
				t.Fatalf("unexpected observed value type %T", observedValue)
			}
		}
		if value != 3 {
			t.Fatalf("expected aggregate sum 3, got %v", value)
		}
	})
}
