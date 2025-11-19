package graph

import (
	"slices"
	"sync"
)

// RuntimeMetrics tracks execution metrics for a running or completed graph.
type RuntimeMetrics struct {
	mu sync.RWMutex

	// CurrentSuperstep tracks the current iteration (superstep) number.
	// In Pregel BSP model, each superstep represents one round of computation.
	CurrentSuperstep int64

	// CompletedNodes lists node names that have finished execution.
	CompletedNodes []string

	// PausedNodes lists node names that are currently paused (e.g., waiting for human input).
	PausedNodes []string

	// ActiveNodes lists node names currently being executed.
	ActiveNodes []string

	// FailedNodes tracks nodes that encountered errors.
	FailedNodes []string

	// TotalMessages counts total messages sent between nodes.
	TotalMessages int64

	// ExecutionTimeNs tracks total execution time in nanoseconds.
	ExecutionTimeNs int64
}

// RuntimeMetricsSnapshot is a read-only snapshot of runtime metrics.
type RuntimeMetricsSnapshot struct {
	CurrentSuperstep int64
	CompletedNodes   []string
	PausedNodes      []string
	ActiveNodes      []string
	FailedNodes      []string
	TotalMessages    int64
	ExecutionTimeNs  int64
}

// NewRuntimeMetrics creates a new runtime metrics tracker.
func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{
		CompletedNodes: make([]string, 0),
		PausedNodes:    make([]string, 0),
		ActiveNodes:    make([]string, 0),
		FailedNodes:    make([]string, 0),
	}
}

// SetSuperstep updates the current superstep number.
func (rm *RuntimeMetrics) SetSuperstep(step int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.CurrentSuperstep = step
}

// AddCompleted marks a node as completed.
func (rm *RuntimeMetrics) AddCompleted(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.CompletedNodes = append(rm.CompletedNodes, nodeName)
	rm.removeActive(nodeName)
}

// AddPaused marks a node as paused.
func (rm *RuntimeMetrics) AddPaused(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if !slices.Contains(rm.PausedNodes, nodeName) {
		rm.PausedNodes = append(rm.PausedNodes, nodeName)
	}
	rm.removeActive(nodeName)
}

// ResumePaused removes a node from the paused list.
func (rm *RuntimeMetrics) ResumePaused(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.PausedNodes = slices.DeleteFunc(rm.PausedNodes, func(s string) bool { return s == nodeName })
}

// AddActive marks a node as currently executing.
func (rm *RuntimeMetrics) AddActive(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if !slices.Contains(rm.ActiveNodes, nodeName) {
		rm.ActiveNodes = append(rm.ActiveNodes, nodeName)
	}
}

// AddFailed marks a node as failed.
func (rm *RuntimeMetrics) AddFailed(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if !slices.Contains(rm.FailedNodes, nodeName) {
		rm.FailedNodes = append(rm.FailedNodes, nodeName)
	}
	rm.removeActive(nodeName)
}

// IncrementMessages increments the total message count.
func (rm *RuntimeMetrics) IncrementMessages(count int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.TotalMessages += count
}

// AddExecutionTime adds to the total execution time.
func (rm *RuntimeMetrics) AddExecutionTime(ns int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.ExecutionTimeNs += ns
}

// removeActive removes a node from the active list (caller must hold lock).
func (rm *RuntimeMetrics) removeActive(nodeName string) {
	rm.ActiveNodes = slices.DeleteFunc(rm.ActiveNodes, func(s string) bool { return s == nodeName })
}

// Snapshot returns a read-only copy of current metrics.
func (rm *RuntimeMetrics) Snapshot() RuntimeMetricsSnapshot {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return RuntimeMetricsSnapshot{
		CurrentSuperstep: rm.CurrentSuperstep,
		CompletedNodes:   slices.Clone(rm.CompletedNodes),
		PausedNodes:      slices.Clone(rm.PausedNodes),
		ActiveNodes:      slices.Clone(rm.ActiveNodes),
		FailedNodes:      slices.Clone(rm.FailedNodes),
		TotalMessages:    rm.TotalMessages,
		ExecutionTimeNs:  rm.ExecutionTimeNs,
	}
}

// Reset clears all metrics.
func (rm *RuntimeMetrics) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.CurrentSuperstep = 0
	rm.CompletedNodes = make([]string, 0)
	rm.PausedNodes = make([]string, 0)
	rm.ActiveNodes = make([]string, 0)
	rm.FailedNodes = make([]string, 0)
	rm.TotalMessages = 0
	rm.ExecutionTimeNs = 0
}
