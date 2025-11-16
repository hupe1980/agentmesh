package state

import (
	"sort"
	"sync"
	"sync/atomic"
)

// =============================================================================
// ExecutionTracker - Monitors vertex execution progress
// =============================================================================

// ExecutionTracker monitors which vertices have completed execution,
// tracks paused vertices, and provides statistics about execution progress.
// This component is responsible for all execution state tracking.
type ExecutionTracker struct {
	mu            sync.RWMutex
	executed      map[string]bool
	paused        map[string]bool
	executedCount atomic.Int64
}

// NewExecutionTracker creates a tracker for monitoring vertex execution.
func NewExecutionTracker() *ExecutionTracker {
	return &ExecutionTracker{
		executed: make(map[string]bool),
		paused:   make(map[string]bool),
	}
}

// MarkExecuted records that a vertex has completed execution.
func (et *ExecutionTracker) MarkExecuted(vertex string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	if !et.executed[vertex] {
		et.executed[vertex] = true
		et.executedCount.Add(1)
	}
}

// WasExecuted checks if a vertex has been executed.
func (et *ExecutionTracker) WasExecuted(vertex string) bool {
	et.mu.RLock()
	defer et.mu.RUnlock()

	return et.executed[vertex]
}

// ExecutedVertices returns a sorted list of all executed vertices.
func (et *ExecutionTracker) ExecutedVertices() []string {
	et.mu.RLock()
	defer et.mu.RUnlock()

	vertices := make([]string, 0, len(et.executed))
	for v := range et.executed {
		vertices = append(vertices, v)
	}
	return vertices
}

// Count returns the number of vertices that have been executed.
func (et *ExecutionTracker) Count() int64 {
	return et.executedCount.Load()
}

// MarkPaused records that a vertex has paused execution (e.g., for human-in-the-loop).
func (et *ExecutionTracker) MarkPaused(vertex string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	et.paused[vertex] = true
}

// IsPaused checks if a vertex is currently paused.
func (et *ExecutionTracker) IsPaused(vertex string) bool {
	et.mu.RLock()
	defer et.mu.RUnlock()

	return et.paused[vertex]
}

// UnpauseVertex removes the paused state from a vertex.
func (et *ExecutionTracker) UnpauseVertex(vertex string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	delete(et.paused, vertex)
}

// PausedVertices returns a sorted list of all paused vertices.
func (et *ExecutionTracker) PausedVertices() []string {
	et.mu.RLock()
	defer et.mu.RUnlock()

	vertices := make([]string, 0, len(et.paused))
	for v := range et.paused {
		vertices = append(vertices, v)
	}
	sort.Strings(vertices)
	return vertices
}

// Reset clears all execution records.
func (et *ExecutionTracker) Reset() {
	et.mu.Lock()
	defer et.mu.Unlock()

	for k := range et.executed {
		delete(et.executed, k)
	}
	for k := range et.paused {
		delete(et.paused, k)
	}
	et.executedCount.Store(0)
}

// SetExecuted marks specific vertices as executed (for resume scenarios).
func (et *ExecutionTracker) SetExecuted(vertices []string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	for _, v := range vertices {
		if !et.executed[v] {
			et.executed[v] = true
			et.executedCount.Add(1)
		}
	}
}

// SetPaused marks specific vertices as paused (for bootstrap scenarios).
func (et *ExecutionTracker) SetPaused(vertices []string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	for _, v := range vertices {
		et.paused[v] = true
	}
}

// =============================================================================
// executionState - Runtime execution state tracking
// =============================================================================

// ExecutionState tracks runtime execution state including completed/paused nodes and superstep counter.
// It provides thread-safe methods for managing node execution lifecycle within the BSP model.
type ExecutionState struct {
	mu        sync.Mutex
	completed map[string]bool
	paused    map[string]bool
	superstep atomic.Int64
}

// NewExecutionState creates a new execution state tracker.
// Exported for backward compatibility with pkg/graph.
func NewExecutionState() *ExecutionState {
	return &ExecutionState{
		completed: make(map[string]bool),
		paused:    make(map[string]bool),
	}
}

// MarkCompleted marks a node as completed and removes it from the paused set.
// Thread-safe. Empty names are ignored.
func (s *ExecutionState) MarkCompleted(name string) {
	if name == "" {
		return
	}
	s.mu.Lock()
	s.completed[name] = true
	delete(s.paused, name)
	s.mu.Unlock()
}

// MarkPaused marks a node as paused (e.g., waiting for human input).
// Thread-safe. Empty names are ignored.
func (s *ExecutionState) MarkPaused(name string) {
	if name == "" {
		return
	}
	s.mu.Lock()
	s.paused[name] = true
	s.mu.Unlock()
}

// ClearPaused removes a node from the paused set (e.g., after human response).
// Thread-safe. Empty names are ignored.
func (s *ExecutionState) ClearPaused(name string) {
	if name == "" {
		return
	}
	s.mu.Lock()
	delete(s.paused, name)
	s.mu.Unlock()
}

// CompletedNames returns a sorted list of all completed node names.
// Returns nil if no nodes are completed. Thread-safe.
func (s *ExecutionState) CompletedNames() []string {
	s.mu.Lock()
	if len(s.completed) == 0 {
		s.mu.Unlock()
		return nil
	}
	names := make([]string, 0, len(s.completed))
	for name, active := range s.completed {
		if active {
			names = append(names, name)
		}
	}
	s.mu.Unlock()
	sort.Strings(names)
	return names
}

// PausedNames returns a sorted list of all paused node names.
// Returns nil if no nodes are paused. Thread-safe.
func (s *ExecutionState) PausedNames() []string {
	s.mu.Lock()
	if len(s.paused) == 0 {
		s.mu.Unlock()
		return nil
	}
	names := make([]string, 0, len(s.paused))
	for name, active := range s.paused {
		if active {
			names = append(names, name)
		}
	}
	s.mu.Unlock()
	sort.Strings(names)
	return names
}

// SetSuperstep updates the current superstep counter.
// Thread-safe. Used by Pregel runtime to track BSP iterations.
func (s *ExecutionState) SetSuperstep(step int64) {
	s.superstep.Store(step)
}

// CurrentSuperstep returns the current superstep counter.
// Thread-safe. Returns 0 if never set.
func (s *ExecutionState) CurrentSuperstep() int64 {
	return s.superstep.Load()
}

// GetPausedMap returns a copy of the paused nodes map (for backward compatibility).
func (s *ExecutionState) GetPausedMap() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyMap := make(map[string]bool, len(s.paused))
	for k, v := range s.paused {
		copyMap[k] = v
	}
	return copyMap
}

// GetCompletedMap returns a copy of the completed nodes map (for backward compatibility).
func (s *ExecutionState) GetCompletedMap() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyMap := make(map[string]bool, len(s.completed))
	for k, v := range s.completed {
		copyMap[k] = v
	}
	return copyMap
}

// SetCompleted directly sets the completed map from a list of node names.
func (s *ExecutionState) SetCompleted(nodes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range nodes {
		s.completed[name] = true
	}
}

// SetPausedList directly sets the paused map from a list of node names.
func (s *ExecutionState) SetPausedList(nodes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range nodes {
		s.paused[name] = true
	}
}
