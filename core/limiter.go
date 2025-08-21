package core

import (
	"fmt"
	"sync"
)

// ModelLimiter enforces a maximum number of allowed model calls per run.
type ModelLimiter struct {
	max   int
	count int
	mu    sync.Mutex
}

// NewModelLimiter creates a new limiter with a max number of calls.
// If max == 0, unlimited calls are allowed.
func NewModelLimiter(max int) *ModelLimiter {
	return &ModelLimiter{max: max}
}

// Increment increases the call counter and returns an error if the limit is exceeded.
func (ml *ModelLimiter) Increment() error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.count++
	if ml.max > 0 && ml.count > ml.max {
		return fmt.Errorf("exceeded max model calls: %d", ml.max)
	}

	return nil
}

// Count returns the current number of calls made.
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
