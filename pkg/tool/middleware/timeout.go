package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

// TimeoutMiddleware enforces execution timeouts for tool calls.
type TimeoutMiddleware struct {
	timeout time.Duration
}

// NewTimeoutMiddleware creates a new timeout middleware.
func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{
		timeout: timeout,
	}
}

// Wrap wraps the tool executor with timeout enforcement.
func (m *TimeoutMiddleware) Wrap(next tool.Executor) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		// Create context with timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		// Execute with timeout
		resultsChan := make(chan struct {
			results []tool.ExecutionResult
			err     error
		}, 1)

		go func() {
			results, err := next.Execute(timeoutCtx, calls)
			resultsChan <- struct {
				results []tool.ExecutionResult
				err     error
			}{results, err}
		}()

		select {
		case <-timeoutCtx.Done():
			// Timeout or cancellation
			if timeoutCtx.Err() == context.DeadlineExceeded {
				// Create error results for all calls
				results := make([]tool.ExecutionResult, len(calls))
				for i := range calls {
					results[i] = tool.ExecutionResult{
						Error: fmt.Errorf("tool execution timeout after %v", m.timeout),
					}
				}
				return results, nil
			}
			return nil, timeoutCtx.Err()
		case result := <-resultsChan:
			return result.results, result.err
		}
	})
}
