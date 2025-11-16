package exec

import (
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
	if !contains(rm.PausedNodes, nodeName) {
		rm.PausedNodes = append(rm.PausedNodes, nodeName)
	}
	rm.removeActive(nodeName)
}

// ResumePaused removes a node from the paused list.
func (rm *RuntimeMetrics) ResumePaused(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.PausedNodes = remove(rm.PausedNodes, nodeName)
}

// AddActive marks a node as currently executing.
func (rm *RuntimeMetrics) AddActive(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if !contains(rm.ActiveNodes, nodeName) {
		rm.ActiveNodes = append(rm.ActiveNodes, nodeName)
	}
}

// AddFailed marks a node as failed.
func (rm *RuntimeMetrics) AddFailed(nodeName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if !contains(rm.FailedNodes, nodeName) {
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

// SetExecutionTime sets the total execution time.
func (rm *RuntimeMetrics) SetExecutionTime(ns int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.ExecutionTimeNs = ns
}

// RuntimeMetricsSnapshot represents a point-in-time snapshot of runtime metrics.
type RuntimeMetricsSnapshot struct {
	CurrentSuperstep int64    `json:"current_superstep"`
	CompletedNodes   []string `json:"completed_nodes"`
	PausedNodes      []string `json:"paused_nodes"`
	ActiveNodes      []string `json:"active_nodes"`
	FailedNodes      []string `json:"failed_nodes"`
	TotalMessages    int64    `json:"total_messages"`
	ExecutionTimeNs  int64    `json:"execution_time_ns"`
}

// Snapshot returns a thread-safe copy of current runtime metrics.
func (rm *RuntimeMetrics) Snapshot() *RuntimeMetricsSnapshot {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Create copies of slices
	completed := make([]string, len(rm.CompletedNodes))
	copy(completed, rm.CompletedNodes)

	paused := make([]string, len(rm.PausedNodes))
	copy(paused, rm.PausedNodes)

	active := make([]string, len(rm.ActiveNodes))
	copy(active, rm.ActiveNodes)

	failed := make([]string, len(rm.FailedNodes))
	copy(failed, rm.FailedNodes)

	return &RuntimeMetricsSnapshot{
		CurrentSuperstep: rm.CurrentSuperstep,
		CompletedNodes:   completed,
		PausedNodes:      paused,
		ActiveNodes:      active,
		FailedNodes:      failed,
		TotalMessages:    rm.TotalMessages,
		ExecutionTimeNs:  rm.ExecutionTimeNs,
	}
}

// removeActive removes a node from the active list (must hold lock).
func (rm *RuntimeMetrics) removeActive(nodeName string) {
	rm.ActiveNodes = remove(rm.ActiveNodes, nodeName)
}

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func remove(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
