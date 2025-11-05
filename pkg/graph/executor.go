package graph

import (
	"context"
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
type Executor interface {
	// Execute runs the graph to completion and returns the final result.
	// This is a blocking call that returns when the graph reaches END or max iterations.
	Execute(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (*InvokeResult, error)

	// Stream executes the graph with real-time event streaming.
	// Returns a channel of execution events that can be consumed by the caller.
	// The event channel is closed when execution completes.
	// Note: StreamEvent type is defined in compiled_graph.go
	Stream(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (<-chan interface{}, <-chan error)

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
		Save(ctx context.Context, checkpoint interface{}) error
		Load(ctx context.Context, runID string) (interface{}, error)
	}

	// CheckpointInterval determines how often to checkpoint (every N supersteps)
	CheckpointInterval int

	// RunID identifies this execution for checkpointing
	RunID string

	// RetryPolicy defines node-level retry behavior
	RetryPolicy interface {
		ShouldRetry(err error, attempt int) bool
		Delay(attempt int) interface{} // time.Duration
	}

	// RateLimiters per-node rate limiting configuration
	RateLimiters map[string]interface{} // map[string]*rate.Limiter

	// Combiner merges multiple messages for the same recipient
	Combiner interface{} // Combiner function
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
	// Messages from the final state
	Messages []message.Message

	// State snapshot at completion
	State map[string]any

	// Metadata about the execution
	Metadata map[string]any
}

// =============================================================================
// ExecutionTracker - Monitors vertex execution progress
// =============================================================================

// ExecutionTracker monitors which vertices have completed execution
// and provides statistics about the execution progress.
type ExecutionTracker struct {
	mu            sync.RWMutex
	executed      map[string]bool
	executedCount atomic.Int64
}

// NewExecutionTracker creates a tracker for monitoring vertex execution.
func NewExecutionTracker() *ExecutionTracker {
	return &ExecutionTracker{
		executed: make(map[string]bool),
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

// Reset clears all execution records.
func (et *ExecutionTracker) Reset() {
	et.mu.Lock()
	defer et.mu.Unlock()

	for k := range et.executed {
		delete(et.executed, k)
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
