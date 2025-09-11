package core

import (
	"fmt"
	"sync"
)

// ModelLimiter enforces a maximum number of model calls per run.
type ModelLimiter struct {
	max   int
	count int
	mu    sync.Mutex
}

// NewModelLimiter creates a limiter with a max number of calls.
// If max == 0, calls are unlimited.
func NewModelLimiter(maxCalls int) *ModelLimiter {
	return &ModelLimiter{max: maxCalls}
}

// Increment increases the call counter and returns an error if the limit is exceeded.
func (ml *ModelLimiter) Increment() error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.count++
	if ml.max > 0 && ml.count > ml.max {
		return fmt.Errorf("%w: max=%d count=%d", ErrMaxModelCallsExceeded, ml.max, ml.count)
	}

	return nil
}

// Count returns the number of calls made so far.
func (ml *ModelLimiter) Count() int {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	return ml.count
}

// Remaining returns how many calls are left before hitting the limit.
func (ml *ModelLimiter) Remaining() int {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if ml.max == 0 {
		return -1 // unlimited
	}

	return ml.max - ml.count
}
