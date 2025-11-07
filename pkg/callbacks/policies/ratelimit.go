package policies

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// rateLimitState tracks request timestamps for rate limiting.
type rateLimitState struct {
	mu       sync.Mutex
	requests []time.Time
}

// RateLimit returns a BeforeModelCallback that enforces request rate limits using a sliding window.
// It maintains closure-based state independent of graph state, tracking request timestamps to
// ensure the request count stays within the specified limit over the time window.
//
// The callback short-circuits with an AI message when the rate limit is exceeded, informing the
// caller when they can retry. Returns nil to continue normal execution when within limits.
//
// Parameters:
//   - maxRequests: Maximum number of requests allowed within the time window
//   - window: Duration of the sliding time window for rate limiting
//
// Example:
//
//	// Allow 100 requests per minute
//	manager.RegisterBeforeModel(policies.RateLimit(100, time.Minute))
//
//	// Allow 10 requests per second
//	manager.RegisterBeforeModel(policies.RateLimit(10, time.Second))
func RateLimit(maxRequests int, window time.Duration) callbacks.BeforeModelCallback {
	state := &rateLimitState{
		requests: make([]time.Time, 0, maxRequests),
	}

	return func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		now := time.Now()

		state.mu.Lock()
		defer state.mu.Unlock()

		// Remove expired requests (sliding window)
		cutoff := now.Add(-window)
		filtered := make([]time.Time, 0, len(state.requests))
		for _, req := range state.requests {
			if req.After(cutoff) {
				filtered = append(filtered, req)
			}
		}
		state.requests = filtered

		// Check limit
		if len(state.requests) >= maxRequests {
			// Calculate when next request will be allowed
			var waitTime time.Duration
			if len(state.requests) > 0 {
				oldestRequest := state.requests[0]
				nextAllowed := oldestRequest.Add(window)
				waitTime = nextAllowed.Sub(now)
			}

			return message.NewAIMessageFromText(
				fmt.Sprintf("Rate limit exceeded: %d requests per %v. Try again in %v.",
					maxRequests, window, waitTime.Round(time.Second)),
			), nil
		}

		// Record request
		state.requests = append(state.requests, now)

		return nil, nil // Continue
	}
}

// PerNodeRateLimit returns a BeforeModelCallback that enforces per-node rate limits.
// Each node gets an independent rate limit counter, allowing different rate limit
// requirements for different nodes in the graph.
//
// The callback short-circuits with a node-specific AI message when the rate limit
// is exceeded. The nodeName is included in error messages for clarity.
//
// Parameters:
//   - nodeName: Name of the node being rate limited (included in error messages)
//   - maxRequests: Maximum requests allowed within the time window
//   - window: Duration of the sliding time window
//
// Example:
//
//	manager.RegisterBeforeModel(policies.PerNodeRateLimit("expensive_node", 10, time.Minute))
func PerNodeRateLimit(nodeName string, maxRequests int, window time.Duration) callbacks.BeforeModelCallback {
	state := &rateLimitState{
		requests: make([]time.Time, 0, maxRequests),
	}

	return func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		now := time.Now()

		state.mu.Lock()
		defer state.mu.Unlock()

		// Remove expired requests
		cutoff := now.Add(-window)
		filtered := make([]time.Time, 0, len(state.requests))
		for _, req := range state.requests {
			if req.After(cutoff) {
				filtered = append(filtered, req)
			}
		}
		state.requests = filtered

		// Check limit
		if len(state.requests) >= maxRequests {
			var waitTime time.Duration
			if len(state.requests) > 0 {
				oldestRequest := state.requests[0]
				nextAllowed := oldestRequest.Add(window)
				waitTime = nextAllowed.Sub(now)
			}

			return message.NewAIMessageFromText(
				fmt.Sprintf("Rate limit exceeded for node '%s': %d requests per %v. Try again in %v.",
					nodeName, maxRequests, window, waitTime.Round(time.Second)),
			), nil
		}

		// Record request
		state.requests = append(state.requests, now)

		return nil, nil
	}
}
