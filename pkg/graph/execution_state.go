package graph

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

type executionState struct {
	mu        sync.Mutex
	completed map[string]bool
	paused    map[string]bool
	superstep int64
}

func newExecutionState() *executionState {
	return &executionState{
		completed: make(map[string]bool),
		paused:    make(map[string]bool),
	}
}

func (s *executionState) markCompleted(name string) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	s.completed[name] = true
	delete(s.paused, name)
	s.mu.Unlock()
}

func (s *executionState) markPaused(name string) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	s.paused[name] = true
	s.mu.Unlock()
}

func (s *executionState) clearPaused(name string) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	delete(s.paused, name)
	s.mu.Unlock()
}

func (s *executionState) completedNames() []string {
	if s == nil {
		return nil
	}
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

func (s *executionState) pausedNames() []string {
	if s == nil {
		return nil
	}
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

func (s *executionState) setSuperstep(step int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.superstep = step
	s.mu.Unlock()
}

func (s *executionState) currentSuperstep() int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	step := s.superstep
	s.mu.Unlock()
	return step
}
