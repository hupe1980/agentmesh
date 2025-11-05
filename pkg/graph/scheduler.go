package graph

import (
	"context"
	"sort"
	"sync"
)

// vertexScheduler orchestrates scheduling by composing three specialized components:
// - TopologyScheduler: DAG dependency tracking
// - ConditionalEvaluator: Conditional edge routing
// - ExecutionTracker: Execution history and paused state
type vertexScheduler struct {
	cg        *CompiledGraph
	mu        sync.RWMutex
	topology  *TopologyScheduler
	evaluator *ConditionalEvaluator
	tracker   *ExecutionTracker
	paused    map[string]bool
}

func newVertexScheduler(cg *CompiledGraph) *vertexScheduler {
	sched := &vertexScheduler{
		cg:        cg,
		topology:  NewTopologyScheduler(cg.incoming),
		evaluator: NewConditionalEvaluator(cg),
		tracker:   NewExecutionTracker(),
		paused:    make(map[string]bool),
	}
	return sched
}

// Ready reports the sorted list of vertices that can execute next.
func (s *vertexScheduler) Ready() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := s.topology.Ready()
	ready := make([]string, 0, len(candidates))

	for _, name := range candidates {
		if s.paused[name] {
			continue
		}
		if !s.evaluator.IsGateOpen(name) {
			continue
		}
		ready = append(ready, name)
	}

	return ready
}

// Bootstrap seeds the scheduler with persisted execution state.
func (s *vertexScheduler) Bootstrap(ctx context.Context, completed, paused []string) {
	s.mu.Lock()
	for _, name := range paused {
		s.paused[name] = true
	}
	s.mu.Unlock()

	s.tracker.SetExecuted(completed)
	s.topology.SetExecuted(completed, s.cg.outgoing)
	s.evaluator.BootstrapOpenGates(completed)

	for _, name := range completed {
		s.applyCompletionForBootstrap(ctx, name)
	}
	s.activateStartConditionals(ctx)
}

// MarkExecuted records that the vertex finished successfully.
func (s *vertexScheduler) MarkExecuted(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tracker.MarkExecuted(name)
	s.topology.MarkExecuted(name, s.cg.outgoing[name])
	delete(s.paused, name)
}

// MarkPaused records that a vertex yielded for external intervention.
func (s *vertexScheduler) MarkPaused(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.paused[name] = true
}

// OnVertexCompleted updates dependent vertices and returns the next ready set.
func (s *vertexScheduler) OnVertexCompleted(ctx context.Context, name string) ([]string, error) {
	// Get downstream vertices (regular edges)
	downstream := s.cg.outgoing[name]

	// Mark topology dependencies satisfied
	s.topology.MarkExecuted(name, downstream)

	// Evaluate conditional edges from this vertex
	conditionalTargets, err := s.evaluator.EvaluateFrom(ctx, name)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all candidates (downstream + conditional)
	candidates := make(map[string]struct{})
	for _, target := range downstream {
		candidates[target] = struct{}{}
	}
	for _, target := range conditionalTargets {
		candidates[target] = struct{}{}
	}

	// Filter to only ready vertices
	toSchedule := make(map[string]struct{})
	for target := range candidates {
		if s.shouldScheduleLocked(target) {
			toSchedule[target] = struct{}{}
		}
	}

	if len(toSchedule) == 0 {
		return nil, nil
	}

	next := make([]string, 0, len(toSchedule))
	for candidate := range toSchedule {
		next = append(next, candidate)
	}
	sort.Strings(next)
	return next, nil
}

// shouldScheduleLocked checks if a vertex is ready to schedule.
func (s *vertexScheduler) shouldScheduleLocked(vertex string) bool {
	// Check if topology allows (dependencies satisfied)
	readyList := s.topology.Ready()
	topologyReady := false
	for _, name := range readyList {
		if name == vertex {
			topologyReady = true
			break
		}
	}
	if !topologyReady {
		return false
	}

	if s.paused[vertex] {
		return false
	}
	if !s.evaluator.IsGateOpen(vertex) {
		return false
	}
	return true
}

// Snapshot returns diagnostic information about scheduler state.
func (s *vertexScheduler) Snapshot() map[string]SchedulerState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make(map[string]SchedulerState)
	topologyReady := s.topology.Ready()
	readySet := make(map[string]bool)
	for _, name := range topologyReady {
		readySet[name] = true
	}

	for _, name := range s.cg.nodeNames {
		snapshot[name] = SchedulerState{
			TopologyReady: readySet[name],
			GateOpen:      s.evaluator.IsGateOpen(name),
			Executed:      s.tracker.WasExecuted(name),
			Paused:        s.paused[name],
		}
	}
	return snapshot
}

// SchedulerState represents diagnostic state for a single vertex.
type SchedulerState struct {
	TopologyReady bool
	GateOpen      bool
	Executed      bool
	Paused        bool
}

// Reset reinitializes all vertex bookkeeping using the compiled graph metadata.
func (s *vertexScheduler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.topology.Reset()
	s.evaluator.Reset()
	s.tracker.Reset()
	for k := range s.paused {
		delete(s.paused, k)
	}

	// Mark START node as executed to activate its downstream vertices
	downstream := s.cg.outgoing[StartNode]
	if len(downstream) > 0 {
		s.topology.MarkExecuted(StartNode, downstream)
	}
}

// EnsureVertexExists reports whether a vertex is tracked by the scheduler.
func (s *vertexScheduler) EnsureVertexExists(name string) bool {
	for _, n := range s.cg.nodeNames {
		if n == name {
			return true
		}
	}
	return false
}

// applyCompletionForBootstrap processes a completed vertex's downstream during bootstrap.
func (s *vertexScheduler) applyCompletionForBootstrap(ctx context.Context, name string) {
	targets := s.cg.outgoing[name]
	for _, target := range targets {
		// No need to mark executed, just ensure downstream can proceed
		_ = target
	}
	// Evaluate any conditional edges from this vertex
	_, _ = s.evaluator.EvaluateFrom(ctx, name)
}

func (s *vertexScheduler) activateStartConditionals(ctx context.Context) {
	// Evaluate conditionals from START node
	_, _ = s.evaluator.EvaluateFrom(ctx, StartNode)
}
