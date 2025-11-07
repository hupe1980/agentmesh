package policies

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Retry Tests

func TestExponentialBackoffRetry_MaxAttempts(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0, // No jitter for predictable testing
	}

	callback := ExponentialBackoffRetry(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// First 3 attempts should retry (return nil error)
	for i := 1; i <= 3; i++ {
		msg, err := callback(ctx, sw, testErr)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i, err)
		}
		if msg != nil {
			t.Fatalf("attempt %d: should retry, got message instead", i)
		}
	}

	// 4th attempt should fail (exceed max attempts)
	msg, err := callback(ctx, sw, testErr)
	if err != nil {
		t.Fatalf("4th attempt: unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("4th attempt: expected failure message")
	}

	// Verify message has content
	parts := msg.Parts()
	if len(parts) == 0 {
		t.Error("retry failure message should have content")
	}
}

func TestExponentialBackoffRetry_BackoffDelay(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       5,
		InitialDelay:      50 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0,
	}

	callback := ExponentialBackoffRetry(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Measure time for first retry
	start := time.Now()
	msg, _ := callback(ctx, sw, testErr)
	elapsed := time.Since(start)

	if msg != nil {
		t.Fatal("should retry, not return message")
	}

	// Should have delayed approximately InitialDelay (50ms)
	// Allow some tolerance
	if elapsed < 40*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Errorf("expected ~50ms delay, got %v", elapsed)
	}
}

func TestRetryWithTimeout_Timeout(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       100, // High number
		InitialDelay:      20 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0,
	}

	timeout := 100 * time.Millisecond
	callback := RetryWithTimeout(config, timeout)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	start := time.Now()

	// Keep retrying until timeout
	var lastMsg message.Message
	for i := 0; i < 10; i++ {
		msg, _ := callback(ctx, sw, testErr)
		lastMsg = msg
		if msg != nil {
			break // Timeout reached
		}
	}

	elapsed := time.Since(start)

	// Should timeout around 100ms
	if elapsed < 80*time.Millisecond {
		t.Errorf("timeout too early: %v", elapsed)
	}

	if lastMsg == nil {
		t.Fatal("expected timeout message")
	}
}

func TestConditionalRetry_Filtering(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = 1 * time.Millisecond

	retryableErr := errors.New("retryable")
	nonRetryableErr := errors.New("non-retryable")

	shouldRetry := func(err error) bool {
		return errors.Is(err, retryableErr)
	}

	callback := ConditionalRetry(config, shouldRetry)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Retryable error should trigger retry
	msg, err := callback(ctx, sw, retryableErr)
	if err != nil {
		t.Fatalf("retryable error: unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatal("retryable error: should retry, not return message")
	}

	// Non-retryable error should propagate immediately
	msg, err = callback(ctx, sw, nonRetryableErr)
	if err == nil {
		t.Fatal("non-retryable error: expected error to propagate")
	}
	if !errors.Is(err, nonRetryableErr) {
		t.Fatalf("expected original error, got: %v", err)
	}
	if msg != nil {
		t.Fatal("non-retryable error: should not return message")
	}
}

func TestCalculateBackoff_Exponential(t *testing.T) {
	config := RetryConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},   // 100 * 2^0
		{1, 200 * time.Millisecond},   // 100 * 2^1
		{2, 400 * time.Millisecond},   // 100 * 2^2
		{3, 800 * time.Millisecond},   // 100 * 2^3
		{4, time.Second},              // 1600ms capped to 1000ms
		{10, time.Second},             // Way over cap
	}

	for _, tt := range tests {
		delay := calculateBackoff(tt.attempt, config)
		// Allow small tolerance for floating point math
		tolerance := 5 * time.Millisecond
		if delay < tt.expected-tolerance || delay > tt.expected+tolerance {
			t.Errorf("calculateBackoff(%d) = %v, want %v", tt.attempt, delay, tt.expected)
		}
	}
}

func TestCalculateBackoff_Jitter(t *testing.T) {
	config := RetryConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.2, // 20% jitter
	}

	// Calculate backoff multiple times and verify jitter is applied
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = calculateBackoff(1, config) // 200ms base * 2.0 = 200ms
	}

	// With jitter, not all delays should be identical
	// (This is probabilistic, but with 10 samples very likely to differ)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	// Note: There's a small chance this could fail if random jitter
	// happens to produce identical values, but it's unlikely
	if allSame {
		t.Log("Warning: All jitter values were identical (unlikely but possible)")
	}

	// All delays should be <= base delay (jitter reduces, not increases)
	baseDelay := 200 * time.Millisecond
	for i, delay := range delays {
		if delay > baseDelay {
			t.Errorf("delay[%d] = %v exceeds base delay %v", i, delay, baseDelay)
		}
		// Should be within jitter range: 80% to 100% of base
		minDelay := time.Duration(float64(baseDelay) * 0.8)
		if delay < minDelay {
			t.Errorf("delay[%d] = %v below minimum %v", i, delay, minDelay)
		}
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 3 {
		t.Errorf("expected default MaxAttempts=3, got %d", config.MaxAttempts)
	}
	if config.InitialDelay != time.Second {
		t.Errorf("expected default InitialDelay=1s, got %v", config.InitialDelay)
	}
	if config.MaxDelay != 30*time.Second {
		t.Errorf("expected default MaxDelay=30s, got %v", config.MaxDelay)
	}
	if config.BackoffMultiplier != 2.0 {
		t.Errorf("expected default BackoffMultiplier=2.0, got %f", config.BackoffMultiplier)
	}
	if config.Jitter != 0.1 {
		t.Errorf("expected default Jitter=0.1, got %f", config.Jitter)
	}
}

func TestExponentialBackoffRetry_ResetsAfterMax(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       2,
		InitialDelay:      1 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0,
	}

	callback := ExponentialBackoffRetry(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Exhaust retries
	callback(ctx, sw, testErr)
	callback(ctx, sw, testErr)
	msg, _ := callback(ctx, sw, testErr) // Should fail
	if msg == nil {
		t.Fatal("expected failure after max attempts")
	}

	// Next error should reset and retry again
	msg, _ = callback(ctx, sw, testErr)
	if msg != nil {
		t.Fatal("expected retry after reset")
	}
}

func TestRetryWithTimeout_StillRespectsMaxAttempts(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       2,
		InitialDelay:      1 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0,
	}

	timeout := time.Hour // Very long timeout
	callback := RetryWithTimeout(config, timeout)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Should still respect max attempts even with long timeout
	callback(ctx, sw, testErr) // Attempt 1
	callback(ctx, sw, testErr) // Attempt 2
	msg, _ := callback(ctx, sw, testErr) // Attempt 3 - should fail

	if msg == nil {
		t.Fatal("expected failure after max attempts despite long timeout")
	}
}

func TestConditionalRetry_RespectsMaxAttempts(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       2,
		InitialDelay:      1 * time.Millisecond,
		MaxDelay:          time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.0,
	}

	shouldRetry := func(err error) bool {
		return true // Always retry
	}

	callback := ConditionalRetry(config, shouldRetry)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Exhaust retries
	callback(ctx, sw, testErr) // Attempt 1
	callback(ctx, sw, testErr) // Attempt 2
	msg, _ := callback(ctx, sw, testErr) // Attempt 3 - should fail

	if msg == nil {
		t.Fatal("expected failure after max attempts")
	}
}
