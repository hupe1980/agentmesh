package graph

import (
	"context"
	"iter"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Executor defines the interface for graph execution strategies.
// This abstraction allows different execution models (Pregel BSP, simple sequential, etc.)
// while keeping the API consistent.
//
// Responsibilities:
//   - Execute graph nodes according to topology and dependencies
//   - Handle superstep coordination and synchronization
//   - Emit execution events for observability
//   - Manage execution lifecycle (start, pause, resume, stop)
//
// Design Goals:
//   - Clean separation between execution strategy and graph topology
//   - Pluggable execution backends (Pregel, Simple, Distributed, etc.)
//   - No state management (delegates to StateManager)
//   - Observable execution via event streams
//
// Usage:
//   - Stream events: for event, err := range executor.Execute(ctx, msgs, opts) { ... }
//   - Block until done: events, err := graph.Collect(executor.Execute(ctx, msgs, opts))
//   - Get last result: lastEvent, err := graph.Last(executor.Execute(ctx, msgs, opts))
type Executor interface {
	// Execute runs the graph and returns an iterator of execution events.
	// Events are emitted as nodes complete execution.
	// The iterator completes when the graph reaches END, max iterations, or an error occurs.
	//
	// Consume patterns:
	//   - Streaming: for event, err := range executor.Execute(ctx, msgs, opts)
	//   - Blocking: events, err := graph.Collect(executor.Execute(ctx, msgs, opts))
	//   - Last only: last, err := graph.Last(executor.Execute(ctx, msgs, opts))
	Execute(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) iter.Seq2[Event, error]

	// Pause pauses execution before the specified node.
	// The node will not execute until Resume() is called.
	Pause(nodeName string)

	// Resume resumes execution of a paused node.
	Resume(nodeName string)

	// IsPaused returns whether the specified node is currently paused.
	IsPaused(nodeName string) bool

	// CurrentSuperstep returns the current superstep number.
	CurrentSuperstep() int64

	// Stats returns execution statistics.
	Stats() ExecutionStats
}

// ExecuteOptions contains configuration for graph execution.
type ExecuteOptions struct {
	// MaxIterations limits the number of supersteps (0 = unlimited)
	MaxIterations int

	// MaxWorkers sets the maximum number of parallel node executions
	MaxWorkers int

	// Checkpointer enables checkpoint persistence
	Checkpointer interface {
		Save(ctx context.Context, checkpoint any) error
		Load(ctx context.Context, runID string) (any, error)
	}

	// CheckpointInterval determines how often to checkpoint (every N supersteps)
	CheckpointInterval int

	// RunID identifies this execution for checkpointing
	RunID string

	// RateLimiters per-node rate limiting configuration
	RateLimiters map[string]any // map[string]*rate.Limiter

	// Combiner merges multiple messages for the same recipient
	Combiner any // Combiner function
}

// ExecutionStats provides runtime execution metrics.
type ExecutionStats struct {
	// Supersteps completed
	Supersteps int64

	// Total vertices executed
	VerticesExecuted int64

	// Total messages sent between vertices
	MessagesSent int64

	// Execution start time
	StartedAt interface{} // time.Time

	// Execution end time (nil if still running)
	CompletedAt interface{} // time.Time
}

// InvokeResult contains the final graph execution result.
type InvokeResult struct {
	// Messages from the final state with execution metadata
	Messages []Event

	// State snapshot at completion
	State map[string]any

	// Metadata about the execution
	Metadata map[string]any
}

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
