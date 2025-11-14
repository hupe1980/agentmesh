package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

var (
	ErrTransient = errors.New("transient network error")
	ErrTimeout   = errors.New("request timeout")
)

func main() {
	fmt.Println("=== AgentMesh Retry Policy Builder Demo ===")
	fmt.Println()

	demonstrateBasicRetry()
	fmt.Println()
	demonstrateBackoffStrategies()
	fmt.Println()
	demonstrateErrorMatching()
}

func demonstrateBasicRetry() {
	fmt.Println("1. Basic Retry with Defaults")
	fmt.Println("   Default: 3 attempts, exponential backoff")
	fmt.Println()

	// Simple policy with defaults
	policy := graph.NewRetryPolicy().Build()

	fmt.Printf("   ✓ MaxAttempts: %d\n", policy.MaxAttempts)
	fmt.Printf("   ✓ Backoff(1): %v\n", policy.Backoff(1))
	fmt.Printf("   ✓ Backoff(2): %v\n", policy.Backoff(2))
	fmt.Printf("   ✓ Backoff(3): %v\n", policy.Backoff(3))
}

func demonstrateBackoffStrategies() {
	fmt.Println("2. Backoff Strategies")
	fmt.Println()

	strategies := []struct {
		name   string
		policy *graph.RetryPolicy
	}{
		{
			name: "Exponential (1s base, 2x multiplier)",
			policy: graph.NewRetryPolicy().
				WithExponentialBackoff(time.Second, 2.0).
				Build(),
		},
		{
			name: "Linear (500ms increments)",
			policy: graph.NewRetryPolicy().
				WithLinearBackoff(500 * time.Millisecond).
				Build(),
		},
		{
			name: "Constant (1s wait)",
			policy: graph.NewRetryPolicy().
				WithConstantBackoff(time.Second).
				Build(),
		},
	}

	for _, s := range strategies {
		fmt.Printf("   %s:\n", s.name)
		for attempt := 1; attempt <= 3; attempt++ {
			fmt.Printf("     Attempt %d: %v\n", attempt, s.policy.Backoff(attempt))
		}
		fmt.Println()
	}

	// Advanced strategies
	fmt.Println("   Advanced Strategies:")

	// Capped exponential
	cappedPolicy := &graph.RetryPolicy{
		MaxAttempts: 10,
		Backoff:     graph.CappedExponentialBackoff(time.Second, 2.0, 10*time.Second),
	}
	fmt.Println("   Capped Exponential (max 10s):")
	for _, attempt := range []int{1, 3, 5, 10} {
		fmt.Printf("     Attempt %d: %v\n", attempt, cappedPolicy.Backoff(attempt))
	}
}

func demonstrateErrorMatching() {
	fmt.Println()
	fmt.Println("3. Selective Error Retry")
	fmt.Println()

	// Only retry specific errors
	policy := graph.NewRetryPolicy().
		WithMaxAttempts(5).
		WithRetryableErrors(ErrTransient, ErrTimeout).
		Build()

	testErrors := []struct {
		err   error
		label string
	}{
		{ErrTransient, "Transient error"},
		{ErrTimeout, "Timeout error"},
		{errors.New("auth failed"), "Auth error"},
	}

	fmt.Println("   Policy: Retry only transient and timeout errors")
	for _, te := range testErrors {
		retryable := policy.Retryable(te.err)
		status := "❌ Won't retry"
		if retryable {
			status = "✅ Will retry"
		}
		fmt.Printf("   %s: %s\n", te.label, status)
	}

	fmt.Println()
	fmt.Println("   💡 See test files for complete graph integration examples")
}
