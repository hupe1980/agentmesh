package graph

import (
	"slices"
	"sync"
)

// RuntimeMetrics tracks execution metrics for a running or completed graph.
// Thread-safe for concurrent access during execution.
type RuntimeMetrics struct {
	mu sync.RWMutex

	// CurrentSuperstep tracks the current iteration (superstep) number.
	// In Pregel BSP model, each superstep represents one round of computation.
	CurrentSuperstep int64

	// CompletedNodes lists node names that have finished execution.
	CompletedNodes []string

	// PausedNodes lists node names that are currently paused (e.g., waiting for human input).
	PausedNodes []string

	// ResumingNodes lists node names that are being resumed from a paused state.
	// These nodes should skip interrupt checks.
	ResumingNodes []string

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
	ResumingNodes    []string
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
		ResumingNodes:  make([]string, 0),
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

// GetSuperstep returns the current superstep number.
func (rm *RuntimeMetrics) GetSuperstep() int64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.CurrentSuperstep
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

// ResumePaused removes a node from the paused list and marks it as resuming.
func (rm *RuntimeMetrics) ResumePaused(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.PausedNodes = slices.DeleteFunc(rm.PausedNodes, func(s string) bool { return s == nodeName })
	if !slices.Contains(rm.ResumingNodes, nodeName) {
		rm.ResumingNodes = append(rm.ResumingNodes, nodeName)
	}
}

// ClearResuming removes a node from the resuming list (after it executes).
func (rm *RuntimeMetrics) ClearResuming(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.ResumingNodes = slices.DeleteFunc(rm.ResumingNodes, func(s string) bool { return s == nodeName })
}

// IsResuming checks if a node is currently being resumed.
func (rm *RuntimeMetrics) IsResuming(nodeName string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return slices.Contains(rm.ResumingNodes, nodeName)
}

// AddActive marks a node as actively executing.
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

// AddMessage increments the message counter.
func (rm *RuntimeMetrics) AddMessage() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.TotalMessages++
}

// AddMessages increments the message counter by n.
func (rm *RuntimeMetrics) AddMessages(n int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.TotalMessages += n
}

// SetExecutionTime sets the total execution time.
func (rm *RuntimeMetrics) SetExecutionTime(ns int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.ExecutionTimeNs = ns
}

// Snapshot creates a read-only snapshot of the current metrics.
func (rm *RuntimeMetrics) Snapshot() RuntimeMetricsSnapshot {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return RuntimeMetricsSnapshot{
		CurrentSuperstep: rm.CurrentSuperstep,
		CompletedNodes:   slices.Clone(rm.CompletedNodes),
		PausedNodes:      slices.Clone(rm.PausedNodes),
		ResumingNodes:    slices.Clone(rm.ResumingNodes),
		ActiveNodes:      slices.Clone(rm.ActiveNodes),
		FailedNodes:      slices.Clone(rm.FailedNodes),
		TotalMessages:    rm.TotalMessages,
		ExecutionTimeNs:  rm.ExecutionTimeNs,
	}
}

// Reset clears all metrics to initial state.
func (rm *RuntimeMetrics) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.CurrentSuperstep = 0
	rm.CompletedNodes = make([]string, 0)
	rm.PausedNodes = make([]string, 0)
	rm.ResumingNodes = make([]string, 0)
	rm.ActiveNodes = make([]string, 0)
	rm.FailedNodes = make([]string, 0)
	rm.TotalMessages = 0
	rm.ExecutionTimeNs = 0
}

// removeActive removes a node from the active list (internal, must hold lock).
func (rm *RuntimeMetrics) removeActive(nodeName string) {
	rm.ActiveNodes = slices.DeleteFunc(rm.ActiveNodes, func(s string) bool { return s == nodeName })
}
