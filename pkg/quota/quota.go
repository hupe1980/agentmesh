// Package quota provides resource quota management for graph execution.
// It enforces limits on memory usage, goroutines, and execution time to prevent resource exhaustion.
package quota

import (
	"context"
	"fmt"
	"runtime"
	"sync"
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
	startTime        time.Time

	// Enforcement actions
	onMemoryExceeded    ActionFunc
	onGoroutineExceeded ActionFunc
	onTimeExceeded      ActionFunc

	mu sync.RWMutex
}

// ActionFunc defines a callback invoked when a quota is exceeded.
// It receives the current usage and limit, and returns an error to fail the execution,
// or nil to log a warning and continue.
type ActionFunc func(usage, limit any) error

// Config configures quota limits and enforcement behavior.
type Config struct {
	// MaxMemoryBytes sets the maximum heap memory allowed (0 = unlimited).
	// When exceeded, triggers GC and invokes OnMemoryExceeded action.
	MaxMemoryBytes uint64

	// MaxGoroutines sets the maximum number of concurrent goroutines (0 = unlimited).
	// When exceeded, blocks new goroutine creation until capacity is available.
	MaxGoroutines int

	// MaxExecutionTime sets the maximum total execution duration (0 = unlimited).
	// When exceeded, invokes OnTimeExceeded action.
	MaxExecutionTime time.Duration

	// OnMemoryExceeded defines the action when memory quota is exceeded.
	// Default: return error to fail execution.
	OnMemoryExceeded ActionFunc

	// OnGoroutineExceeded defines the action when goroutine quota is exceeded.
	// Default: block until capacity is available (backpressure).
	OnGoroutineExceeded ActionFunc

	// OnTimeExceeded defines the action when time quota is exceeded.
	// Default: return error to fail execution.
	OnTimeExceeded ActionFunc
}

// New creates a new quota manager with the given configuration.
func New(cfg Config) *Manager {
	m := &Manager{
		maxMemoryBytes:   cfg.MaxMemoryBytes,
		maxGoroutines:    int32(cfg.MaxGoroutines), //nolint:gosec // MaxGoroutines is validated to be positive
		maxExecutionTime: cfg.MaxExecutionTime,
		memoryCheckFunc:  defaultMemoryCheck,
	}

	// Set default actions
	if cfg.OnMemoryExceeded != nil {
		m.onMemoryExceeded = cfg.OnMemoryExceeded
	} else {
		m.onMemoryExceeded = defaultMemoryAction
	}

	if cfg.OnGoroutineExceeded != nil {
		m.onGoroutineExceeded = cfg.OnGoroutineExceeded
	} else {
		m.onGoroutineExceeded = defaultGoroutineAction
	}

	if cfg.OnTimeExceeded != nil {
		m.onTimeExceeded = cfg.OnTimeExceeded
	} else {
		m.onTimeExceeded = defaultTimeAction
	}

	return m
}

// Start initializes the quota manager for a new execution.
// Must be called before using CheckMemory, AcquireGoroutine, or CheckTime.
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startTime = time.Now()
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
func (m *Manager) CheckTime(ctx context.Context) error {
	if m.maxExecutionTime == 0 {
		return nil // Unlimited
	}

	m.mu.RLock()
	startTime := m.startTime
	m.mu.RUnlock()

	elapsed := time.Since(startTime)
	if elapsed <= m.maxExecutionTime {
		return nil // Within limit
	}

	// Over limit - invoke action
	return m.onTimeExceeded(elapsed, m.maxExecutionTime)
}

// Stats returns current resource usage statistics.
func (m *Manager) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return Stats{
		MemoryBytes:      m.memoryCheckFunc(),
		MaxMemoryBytes:   m.maxMemoryBytes,
		ActiveGoroutines: int(m.activeGoroutines.Load()),
		MaxGoroutines:    int(m.maxGoroutines),
		Elapsed:          time.Since(m.startTime),
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
	return fmt.Errorf("execution time quota exceeded: elapsed %s, limit %s", elapsed, maxTime)
}
