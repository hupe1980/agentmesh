package core

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test unlimited mode (max == 0) allows any number of increments without error.
func TestModelLimiter_Unlimited(t *testing.T) {
	ml := NewModelLimiter(0)

	for range 5 {
		err := ml.Increment()
		assert.NoError(t, err, "unlimited limiter should not error")
	}

	assert.Equal(t, 5, ml.Count(), "count should reflect total increments")
	assert.Equal(t, -1, ml.Remaining(), "remaining should be -1 for unlimited")
}

// Test limit is enforced and Remaining/Count behave as expected.
func TestModelLimiter_LimitEnforced(t *testing.T) {
	ml := NewModelLimiter(3)

	// First three should pass
	require.NoError(t, ml.Increment())
	require.NoError(t, ml.Increment())
	require.NoError(t, ml.Increment())

	assert.Equal(t, 3, ml.Count())
	assert.Equal(t, 0, ml.Remaining())

	// Next one exceeds the limit
	err := ml.Increment()
	assert.Error(t, err, "should error when exceeding limit")

	// Count still increases even when exceeding the limit by design
	assert.Equal(t, 4, ml.Count())
	assert.Less(t, ml.Remaining(), 0, "remaining goes negative when exceeded")
}

// Test concurrent increments in unlimited mode remain race-free and accurate.
func TestModelLimiter_ConcurrentUnlimited(t *testing.T) {
	ml := NewModelLimiter(0)

	const n = 100
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ml.Increment(); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()

	close(errs)

	// No errors expected in unlimited mode
	for err := range errs {
		assert.NoError(t, err)
	}

	assert.Equal(t, n, ml.Count())
	assert.Equal(t, -1, ml.Remaining())
}

// Test concurrent increments with a limit: some should error once limit is exceeded.
func TestModelLimiter_ConcurrentWithLimit(t *testing.T) {
	const maxLimit = 50
	const calls = 100

	ml := NewModelLimiter(maxLimit)

	var wg sync.WaitGroup
	errs := make(chan error, calls)

	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- ml.Increment()
		}()
	}

	wg.Wait()

	close(errs)

	// Count errors where limit was exceeded
	errorCount := 0
	for err := range errs {
		if err != nil {
			errorCount++
		}
	}

	// Expect exactly calls - maxLimit errors due to enforcement
	assert.Equal(t, calls-maxLimit, errorCount, "number of errors should equal calls beyond the limit")

	// Count reflects total attempts
	assert.Equal(t, calls, ml.Count())
	assert.Less(t, ml.Remaining(), 0)
}
