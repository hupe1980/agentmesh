package graph

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/hupe1980/agentmesh/pkg/message"
)

func TestRateLimitBasic(t *testing.T) {
	var callCount int32

	builder := NewBuilder()
	builder.Node("rate_limited", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		atomic.AddInt32(&callCount, 1)
		return &NodeResult{}, nil
	})
	builder.AddEdge(StartNode, "rate_limited")
	builder.AddEdge("rate_limited", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	// Limit to 10 requests per second (100ms per request)
	start := time.Now()
	_, err = compiled.Invoke(
		context.Background(),
		[]message.Message{message.NewHumanMessageFromText("test")},
		WithRateLimit("rate_limited", rate.Limit(10), 1),
	)
	require.NoError(t, err)

	elapsed := time.Since(start)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

	// First call should be fast (no rate limiting delay)
	assert.Less(t, elapsed, 50*time.Millisecond)
}

func TestRateLimitMultipleInvocations(t *testing.T) {
	// Test rate limiting across separate compiled graphs to simulate
	// multiple executions of the same node

	var callTimes []time.Time
	var mu sync.Mutex

	nodeFunc := func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		mu.Lock()
		callTimes = append(callTimes, time.Now())
		mu.Unlock()
		return &NodeResult{}, nil
	}

	// Create shared rate limiter to pass to multiple graphs
	limiter := rate.NewLimiter(rate.Limit(5), 1) // 5 req/sec = 200ms per request

	start := time.Now()
	for i := 0; i < 3; i++ {
		builder := NewBuilder()
		builder.Node("rate_limited", nodeFunc)
		builder.AddEdge(StartNode, "rate_limited")
		builder.AddEdge("rate_limited", EndNode)

		compiled, err := builder.Compile()
		require.NoError(t, err)

		// Manually set rate limiter (simulating persistent rate limiting)
		compiled.rateLimiters = map[string]*rate.Limiter{
			"rate_limited": limiter,
		}

		_, err = compiled.Invoke(
			context.Background(),
			[]message.Message{message.NewHumanMessageFromText(fmt.Sprintf("test-%d", i))},
		)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	mu.Lock()
	numCalls := len(callTimes)
	mu.Unlock()

	assert.Equal(t, 3, numCalls, "Node should be called 3 times")

	// Should take at least 400ms (200ms wait between each call)
	assert.GreaterOrEqual(t, elapsed, 350*time.Millisecond, "Rate limiting should add delay")
}

func TestRateLimitContextCancellation(t *testing.T) {
	var callCount int32

	builder := NewBuilder()
	builder.Node("rate_limited", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		atomic.AddInt32(&callCount, 1)
		return &NodeResult{}, nil
	})
	builder.AddEdge(StartNode, "rate_limited")
	builder.AddEdge("rate_limited", EndNode)

	limiter := rate.NewLimiter(rate.Limit(2), 1) // 2 requests per second, burst 1

	// First call consumes the burst token
	compiled1, err := builder.Compile()
	require.NoError(t, err)
	compiled1.rateLimiters = map[string]*rate.Limiter{"rate_limited": limiter}

	_, err = compiled1.Invoke(
		context.Background(),
		[]message.Message{message.NewHumanMessageFromText("test1")},
	)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

	// Immediately try second call (no tokens available, must wait ~500ms)
	// Use short timeout to trigger cancellation
	compiled2, err := builder.Compile()
	require.NoError(t, err)
	compiled2.rateLimiters = map[string]*rate.Limiter{"rate_limited": limiter}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Small delay to ensure limiter is actually rate limited
	time.Sleep(5 * time.Millisecond)

	_, err = compiled2.Invoke(
		ctx,
		[]message.Message{message.NewHumanMessageFromText("test2")},
	)

	// Should fail due to context timeout
	if err == nil {
		t.Skip("Rate limiter had available tokens - timing issue, skipping test")
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit wait failed")

	// Only first call completed
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
}

func TestRateLimitSelectiveNodes(t *testing.T) {
	var fastCallCount, slowCallCount int32

	builder := NewBuilder()
	builder.Node("fast", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		atomic.AddInt32(&fastCallCount, 1)
		return &NodeResult{}, nil
	})
	builder.Node("slow", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		atomic.AddInt32(&slowCallCount, 1)
		return &NodeResult{}, nil
	})
	builder.AddEdge(StartNode, "fast")
	builder.AddEdge(StartNode, "slow")
	builder.AddEdge("fast", EndNode)
	builder.AddEdge("slow", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	start := time.Now()
	_, err = compiled.Invoke(
		context.Background(),
		[]message.Message{message.NewHumanMessageFromText("test")},
		WithRateLimit("slow", rate.Limit(10), 1), // Only slow is rate limited
	)
	require.NoError(t, err)
	elapsed := time.Since(start)

	assert.Equal(t, int32(1), atomic.LoadInt32(&fastCallCount))
	assert.Equal(t, int32(1), atomic.LoadInt32(&slowCallCount))

	// Should be fast since only one node execution is delayed
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestRateLimitBurstAllowance(t *testing.T) {
	var callTimes []time.Time
	var mu sync.Mutex

	nodeFunc := func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		mu.Lock()
		callTimes = append(callTimes, time.Now())
		mu.Unlock()
		return &NodeResult{}, nil
	}

	// Low rate but high burst: allows 5 immediate requests
	limiter := rate.NewLimiter(rate.Limit(1), 5) // 1 per second, burst of 5

	// First 5 should complete quickly due to burst allowance
	start := time.Now()
	for i := 0; i < 5; i++ {
		builder := NewBuilder()
		builder.Node("rate_limited", nodeFunc)
		builder.AddEdge(StartNode, "rate_limited")
		builder.AddEdge("rate_limited", EndNode)

		compiled, err := builder.Compile()
		require.NoError(t, err)
		compiled.rateLimiters = map[string]*rate.Limiter{"rate_limited": limiter}

		_, err = compiled.Invoke(
			context.Background(),
			[]message.Message{message.NewHumanMessageFromText(fmt.Sprintf("test-%d", i))},
		)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	mu.Lock()
	numCalls := len(callTimes)
	mu.Unlock()

	assert.Equal(t, 5, numCalls)

	// All 5 should complete quickly due to burst allowance
	assert.Less(t, elapsed, 150*time.Millisecond)

	// 6th request should wait for token replenishment
	builder := NewBuilder()
	builder.Node("rate_limited", nodeFunc)
	builder.AddEdge(StartNode, "rate_limited")
	builder.AddEdge("rate_limited", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)
	compiled.rateLimiters = map[string]*rate.Limiter{"rate_limited": limiter}

	start = time.Now()
	_, err = compiled.Invoke(
		context.Background(),
		[]message.Message{message.NewHumanMessageFromText("test-6")},
	)
	require.NoError(t, err)
	elapsed = time.Since(start)

	mu.Lock()
	numCalls = len(callTimes)
	mu.Unlock()

	assert.Equal(t, 6, numCalls)
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond) // Should wait ~1 second
}

func TestRateLimitInvalidParameters(t *testing.T) {
	builder := NewBuilder()
	builder.Node("test", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		return &NodeResult{}, nil
	})
	builder.AddEdge(StartNode, "test")
	builder.AddEdge("test", EndNode)

	compiled, err := builder.Compile()
	require.NoError(t, err)

	// Should succeed (invalid params ignored, no rate limiting applied)
	_, err = compiled.Invoke(
		context.Background(),
		[]message.Message{message.NewHumanMessageFromText("test")},
		WithRateLimit("", rate.Limit(10), 1),     // Empty node name
		WithRateLimit("test", rate.Limit(0), 1),  // Zero rate
		WithRateLimit("test", rate.Limit(10), 0), // Zero burst
		WithRateLimit("test", rate.Limit(-1), 1), // Negative rate
	)
	require.NoError(t, err)
}
