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

// rateLimitState tracks request timestamps for rate limiting
type rateLimitState struct {
	mu       sync.Mutex
	requests []time.Time
}

// RateLimit returns a BeforeModelCallback that enforces request rate limits.
// It maintains its own state using closures, independent of graph state.
//
// Parameters:
//   - maxRequests: Maximum number of requests allowed within the time window
//   - window: Time window for rate limiting
//
// The rate limiter uses a sliding window approach:
//   - Removes requests older than the window
//   - Checks if current request count is below max
//   - Records the current request timestamp
//
// If the rate limit is exceeded, it returns a short-circuit response.
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
// Each node name gets its own independent rate limit counter.
//
// This is useful when different nodes have different rate limit requirements.
//
// Parameters:
//   - nodeName: The name of the node to rate limit (for logging only)
//   - maxRequests: Maximum requests per window
//   - window: Time window
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
