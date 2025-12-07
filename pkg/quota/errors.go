// Package quota provides structured errors for quota violations.
package quota

import (
	"fmt"
	"time"
)

// =============================================================================
// Structured Errors
// =============================================================================

// MemoryQuotaExceededError represents a memory quota violation.
// Use errors.As to extract the UsageBytes and LimitBytes fields for programmatic handling.
type MemoryQuotaExceededError struct {
	// UsageBytes is the current memory usage in bytes.
	UsageBytes uint64
	// LimitBytes is the maximum allowed memory in bytes.
	LimitBytes uint64
}

// Error implements the error interface.
func (e *MemoryQuotaExceededError) Error() string {
	return fmt.Sprintf("quota: memory exceeded: using %d bytes, limit %d bytes", e.UsageBytes, e.LimitBytes)
}

// Is enables comparison with sentinel errors.
func (e *MemoryQuotaExceededError) Is(target error) bool {
	_, ok := target.(*MemoryQuotaExceededError)
	return ok
}

// GoroutineQuotaExceededError represents a goroutine quota violation.
// Use errors.As to extract the ActiveCount and LimitCount fields for programmatic handling.
type GoroutineQuotaExceededError struct {
	// ActiveCount is the current number of active goroutines.
	ActiveCount int
	// LimitCount is the maximum allowed goroutines.
	LimitCount int
}

// Error implements the error interface.
func (e *GoroutineQuotaExceededError) Error() string {
	return fmt.Sprintf("quota: goroutine limit exceeded: %d active, limit %d", e.ActiveCount, e.LimitCount)
}

// Is enables comparison with sentinel errors.
func (e *GoroutineQuotaExceededError) Is(target error) bool {
	_, ok := target.(*GoroutineQuotaExceededError)
	return ok
}

// ExecutionTimeExceededError represents an execution time quota violation.
// Use errors.As to extract the Elapsed and MaxTime fields for programmatic handling.
type ExecutionTimeExceededError struct {
	// Elapsed is the actual execution duration.
	Elapsed time.Duration
	// MaxTime is the maximum allowed execution duration.
	MaxTime time.Duration
}

// Error implements the error interface.
func (e *ExecutionTimeExceededError) Error() string {
	return fmt.Sprintf("quota: execution time exceeded: %v (limit: %v)", e.Elapsed, e.MaxTime)
}

// Is enables comparison with sentinel errors.
func (e *ExecutionTimeExceededError) Is(target error) bool {
	_, ok := target.(*ExecutionTimeExceededError)
	return ok
}
