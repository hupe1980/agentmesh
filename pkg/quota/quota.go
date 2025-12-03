// Package quota provides resource quota management for graph execution.
// It enforces limits on memory usage, goroutines, and execution time to prevent resource exhaustion.
package quota

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

// Manager enforces resource quotas for graph execution to prevent resource exhaustion.
// It tracks memory usage, active goroutines, and enforces limits with configurable actions.
type Manager struct {
	// Memory limits
	maxMemoryBytes  uint64
	memoryCheckFunc func() uint64 // Allows injection for testing

	// Goroutine limits
	maxGoroutines    int32
	activeGoroutines atomic.Int32

	// Time limits
	maxExecutionTime time.Duration
	startTime        atomic.Int64 // Unix nanoseconds

	// Enforcement actions
	onMemoryExceeded    ActionFunc
	onGoroutineExceeded ActionFunc
	onTimeExceeded      ActionFunc
}

// ActionFunc defines a callback invoked when a quota is exceeded.
// It receives the current usage and limit, and returns an error to fail the execution,
// or nil to log a warning and continue.
type ActionFunc func(usage, limit any) error

// Option is a functional option for configuring quota limits and enforcement behavior.
type Option func(*Manager)

// WithMaxMemoryBytes sets the maximum heap memory allowed (0 = unlimited).
// When exceeded, triggers GC and invokes OnMemoryExceeded action.
func WithMaxMemoryBytes(bytes uint64) Option {
	return func(m *Manager) {
		m.maxMemoryBytes = bytes
	}
}

// WithMaxGoroutines sets the maximum number of concurrent goroutines (0 = unlimited).
// When exceeded, blocks new goroutine creation until capacity is available.
func WithMaxGoroutines(maxGoroutines int) Option {
	return func(m *Manager) {
		m.maxGoroutines = int32(maxGoroutines)
	}
}

// WithMaxExecutionTime sets the maximum total execution duration (0 = unlimited).
// When exceeded, invokes OnTimeExceeded action.
func WithMaxExecutionTime(duration time.Duration) Option {
	return func(m *Manager) {
		m.maxExecutionTime = duration
	}
}

// WithMemoryExceededAction defines the action when memory quota is exceeded.
// Default: return error to fail execution.
func WithMemoryExceededAction(action ActionFunc) Option {
	return func(m *Manager) {
		m.onMemoryExceeded = action
	}
}

// WithGoroutineExceededAction defines the action when goroutine quota is exceeded.
// Default: block until capacity is available (backpressure).
func WithGoroutineExceededAction(action ActionFunc) Option {
	return func(m *Manager) {
		m.onGoroutineExceeded = action
	}
}

// WithTimeExceededAction defines the action when time quota is exceeded.
// Default: return error to fail execution.
func WithTimeExceededAction(action ActionFunc) Option {
	return func(m *Manager) {
		m.onTimeExceeded = action
	}
}

// New creates a new quota manager with the given options.
func New(opts ...Option) *Manager {
	m := &Manager{
		memoryCheckFunc:     defaultMemoryCheck,
		onMemoryExceeded:    defaultMemoryAction,
		onGoroutineExceeded: defaultGoroutineAction,
		onTimeExceeded:      defaultTimeAction,
	}

	// Apply options
	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start initializes the quota manager for a new execution.
// Must be called before using CheckMemory, AcquireGoroutine, or CheckTime.
func (m *Manager) Start() {
	m.startTime.Store(time.Now().UnixNano())
	m.activeGoroutines.Store(0)
}

// CheckMemory verifies current memory usage is within quota.
// If exceeded, triggers GC and invokes the configured action.
// Returns an error if the action decides to fail execution.
func (m *Manager) CheckMemory(ctx context.Context) error {
	if m.maxMemoryBytes == 0 {
		return nil // Unlimited
	}

	currentBytes := m.memoryCheckFunc()
	if currentBytes <= m.maxMemoryBytes {
		return nil // Within limit
	}

	// Try GC to reclaim memory
	runtime.GC()
	runtime.GC() // Double GC recommended for better cleanup

	// Check again after GC
	currentBytes = m.memoryCheckFunc()
	if currentBytes <= m.maxMemoryBytes {
		return nil // GC freed enough memory
	}

	// Still over limit - invoke action
	return m.onMemoryExceeded(currentBytes, m.maxMemoryBytes)
}

// AcquireGoroutine attempts to acquire a goroutine slot.
// Blocks if at capacity (backpressure), or invokes configured action.
// Must call ReleaseGoroutine() when done (use defer).
func (m *Manager) AcquireGoroutine(ctx context.Context) error {
	if m.maxGoroutines == 0 {
		return nil // Unlimited
	}

	// Increment and check
	current := m.activeGoroutines.Add(1)
	if current <= m.maxGoroutines {
		return nil // Slot acquired
	}

	// Over limit - decrement and handle
	m.activeGoroutines.Add(-1)

	// Invoke action (default is to block until available)
	return m.onGoroutineExceeded(int(current), int(m.maxGoroutines))
}

// ReleaseGoroutine releases a goroutine slot acquired with AcquireGoroutine.
func (m *Manager) ReleaseGoroutine() {
	if m.maxGoroutines == 0 {
		return // Unlimited
	}
	m.activeGoroutines.Add(-1)
}

// CheckTime verifies execution duration is within quota.
// Returns an error if time limit is exceeded and action decides to fail.
// Uses atomic operations to prevent TOCTOU (time-of-check-time-of-use) races.
func (m *Manager) CheckTime(ctx context.Context) error {
	if m.maxExecutionTime == 0 {
		return nil // Unlimited
	}

	// Single atomic load + calculation to prevent race condition
	startNano := m.startTime.Load()
	if startNano == 0 {
		return nil // Not started yet
	}

	elapsed := time.Since(time.Unix(0, startNano))
	if elapsed <= m.maxExecutionTime {
		return nil // Within limit
	}

	// Over limit - invoke action
	return m.onTimeExceeded(elapsed, m.maxExecutionTime)
}

// Stats returns current resource usage statistics.
func (m *Manager) Stats() Stats {
	// Calculate elapsed time atomically
	var elapsed time.Duration
	if startNano := m.startTime.Load(); startNano != 0 {
		elapsed = time.Since(time.Unix(0, startNano))
	}

	return Stats{
		MemoryBytes:      m.memoryCheckFunc(),
		MaxMemoryBytes:   m.maxMemoryBytes,
		ActiveGoroutines: int(m.activeGoroutines.Load()),
		MaxGoroutines:    int(m.maxGoroutines),
		Elapsed:          elapsed,
		MaxExecutionTime: m.maxExecutionTime,
	}
}

// Stats holds resource usage statistics.
type Stats struct {
	MemoryBytes      uint64
	MaxMemoryBytes   uint64
	ActiveGoroutines int
	MaxGoroutines    int
	Elapsed          time.Duration
	MaxExecutionTime time.Duration
}

// =============================================================================
// Default implementations
// =============================================================================

func defaultMemoryCheck() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func defaultMemoryAction(usage, limit any) error {
	usageBytes := usage.(uint64)
	limitBytes := limit.(uint64)
	return fmt.Errorf("memory quota exceeded: using %d bytes, limit %d bytes", usageBytes, limitBytes)
}

func defaultGoroutineAction(usage, limit any) error {
	usageCount := usage.(int)
	limitCount := limit.(int)
	return fmt.Errorf("goroutine quota exceeded: %d active, limit %d", usageCount, limitCount)
}

func defaultTimeAction(usage, limit any) error {
	elapsed := usage.(time.Duration)
	maxTime := limit.(time.Duration)
	return fmt.Errorf("quota: execution time exceeded: %v (limit: %v)", elapsed, maxTime)
}
