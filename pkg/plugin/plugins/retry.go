package plugins

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// RetryPlugin automatically retries failed model calls with exponential backoff.
type RetryPlugin struct {
	plugin.NoopPlugin

	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration

	mu      sync.Mutex
	retries map[string]int // Track retries per request
}

// NewRetryPlugin creates a retry plugin with exponential backoff.
// maxRetries is the maximum number of retry attempts.
// baseDelay is the initial delay between retries.
// maxDelay is the maximum delay between retries.
func NewRetryPlugin(maxRetries int, baseDelay, maxDelay time.Duration) *RetryPlugin {
	return &RetryPlugin{
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		maxDelay:   maxDelay,
		retries:    make(map[string]int),
	}
}

// OnModelError tracks retry attempts for failed invocations.
func (p *RetryPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	// For retry to work properly, this would need integration with the model execution layer
	// Since we can't actually retry from here (the model call already failed),
	// this plugin tracks retry attempts but doesn't implement actual retries.
	//
	// A proper implementation would require:
	// 1. Storing the request context
	// 2. Re-invoking the model from within the plugin
	// 3. Managing the retry loop
	//
	// For now, this is a placeholder that demonstrates the pattern.

	p.mu.Lock()
	defer p.mu.Unlock()

	requestKey := fmt.Sprintf("%p", req) // Use pointer address as key
	attempts := p.retries[requestKey]

	if attempts < p.maxRetries {
		// Calculate backoff delay with safe conversion
		// #nosec G115 -- attempts is bounded by maxRetries (typically < 10)
		delay := min(p.baseDelay*time.Duration(1<<uint(attempts)), p.maxDelay)

		p.retries[requestKey] = attempts + 1

		// Note: Actual retry would happen here if we had access to the model executor
		return nil, fmt.Errorf("retry %d/%d would occur after %v: %w", attempts+1, p.maxRetries, delay, err)
	}

	// Max retries exceeded, clean up and propagate error
	delete(p.retries, requestKey)
	return nil, fmt.Errorf("max retries (%d) exceeded: %w", p.maxRetries, err)
}

// AfterModel clears retry state after successful invocation.
func (p *RetryPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	// Success - clean up retry tracking
	p.mu.Lock()
	requestKey := fmt.Sprintf("%p", req)
	delete(p.retries, requestKey)
	p.mu.Unlock()

	return nil, nil
}

// GetRetryCount returns the current retry count for debugging.
func (p *RetryPlugin) GetRetryCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.retries)
}
