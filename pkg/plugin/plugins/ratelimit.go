package plugins

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// RateLimitPlugin limits the rate of model calls within a time window.
// It uses a simple sliding window algorithm to track request counts.
type RateLimitPlugin struct {
	plugin.NoopPlugin

	maxRequests int
	window      time.Duration

	mu       sync.Mutex
	requests []time.Time
}

// NewRateLimitPlugin creates a rate limiting plugin.
// maxRequests is the maximum number of requests allowed within the time window.
// window is the duration of the sliding time window.
func NewRateLimitPlugin(maxRequests int, window time.Duration) *RateLimitPlugin {
	return &RateLimitPlugin{
		maxRequests: maxRequests,
		window:      window,
		requests:    []time.Time{},
	}
}

// BeforeModel enforces rate limiting before model invocation.
func (p *RateLimitPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.window)

	// Remove old requests outside the window
	validRequests := []time.Time{}
	for _, t := range p.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	p.requests = validRequests

	// Check if we're at the limit
	if len(p.requests) >= p.maxRequests {
		return nil, fmt.Errorf("rate limit exceeded: %d requests in %v", p.maxRequests, p.window)
	}

	// Record this request
	p.requests = append(p.requests, now)

	return nil, nil
}

// GetCurrentRate returns the number of requests in the current window.
func (p *RateLimitPlugin) GetCurrentRate() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.window)

	count := 0
	for _, t := range p.requests {
		if t.After(cutoff) {
			count++
		}
	}

	return count
}
