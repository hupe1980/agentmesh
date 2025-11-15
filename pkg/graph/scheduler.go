package graph

import (
	"context"
	"sort"
	"sync"
)

// vertexScheduler orchestrates scheduling by composing three specialized components:
// - TopologyScheduler: DAG dependency tracking (immutable topology)
// - ConditionalEvaluator: Conditional edge routing (state-based routing)
// - ExecutionTracker: Execution history and paused state (mutable execution state)
//
// This separation follows the Single Responsibility Principle:
// - Topology: Pure DAG structure and dependencies
// - Evaluator: Stateful conditional logic
// - Tracker: Execution progress and control flow
//
// Thread Safety:
//   - All methods acquire appropriate locks (RLock for reads, Lock for writes)
//   - Component methods (topology, evaluator, tracker) have internal locking
//   - Safe for concurrent calls to Ready(), MarkExecuted(), etc.
type vertexScheduler struct {
	cg        Structure
	mu        sync.RWMutex
	topology  *TopologyScheduler
	evaluator *ConditionalEvaluator
	tracker   *ExecutionTracker
}

func newVertexScheduler(cg Structure) *vertexScheduler {
	sched := &vertexScheduler{
		cg:        cg,
		topology:  NewTopologyScheduler(cg.Incoming()),
		evaluator: NewConditionalEvaluator(cg),
		tracker:   NewExecutionTracker(),
	}
	return sched
}

// Ready reports the sorted list of vertices that can execute next.
// A vertex is ready if:
//  1. All topology dependencies are satisfied (TopologyScheduler)
//  2. It is not paused (ExecutionTracker)
//  3. Its conditional gate is open (ConditionalEvaluator)
func (s *vertexScheduler) Ready() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := s.topology.Ready()
	ready := make([]string, 0, len(candidates))

	for _, name := range candidates {
		if s.tracker.IsPaused(name) {
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
// This is used for checkpoint resume scenarios.
func (s *vertexScheduler) Bootstrap(ctx context.Context, completed, paused []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set paused state in tracker
	s.tracker.SetPaused(paused)

	if len(completed) > 0 {
		// Resuming from checkpoint: mark START and all completed nodes as executed
		s.tracker.MarkExecuted(StartNode)
		s.topology.MarkExecuted(StartNode, s.cg.Outgoing()[StartNode])

		// Set executed state in all components
		s.tracker.SetExecuted(completed)
		s.topology.SetExecuted(completed, s.cg.Outgoing())
		s.evaluator.BootstrapOpenGates(completed)

		// Apply completion logic for bootstrap
		for _, name := range completed {
			s.applyCompletionForBootstrap(ctx, name)
		}
	} else {
		// Normal execution (not resuming): activate START's downstream nodes
		// This is the same logic as Reset() - mark START as executed to activate its children
		downstream := s.cg.Outgoing()[StartNode]
		if len(downstream) > 0 {
			s.topology.MarkExecuted(StartNode, downstream)
		}

		// Also activate START conditionals
		s.activateStartConditionals(ctx)
	}
}

// MarkExecuted records that the vertex finished successfully.
// This updates all three components: topology, tracker, and unpause.
func (s *vertexScheduler) MarkExecuted(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tracker.MarkExecuted(name)
	s.topology.MarkExecuted(name, s.cg.Outgoing()[name])
	s.tracker.UnpauseVertex(name)
}

// MarkPaused records that a vertex yielded for external intervention.
// This is used for human-in-the-loop workflows.
func (s *vertexScheduler) MarkPaused(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tracker.MarkPaused(name)
}

// OnVertexCompleted updates dependent vertices and returns the next ready set.
func (s *vertexScheduler) OnVertexCompleted(ctx context.Context, name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get downstream vertices (regular edges)
	downstream := s.cg.Outgoing()[name]

	// Mark topology dependencies satisfied
	s.topology.MarkExecuted(name, downstream)

	// Evaluate conditional edges from this vertex
	conditionalTargets, err := s.evaluator.EvaluateFrom(ctx, name)
	if err != nil {
		return nil, err
	}

	// Collect all candidates (downstream + conditional)
	candidates := make(map[string]struct{})
	for _, target := range downstream {
		candidates[target] = struct{}{}
	}
	for _, target := range conditionalTargets {
		candidates[target] = struct{}{}
	}

	// In BSP model with message-based propagation, we return ALL downstream nodes
	// so messages are sent to them. The Pregel runtime's frontier logic will handle
	// scheduling nodes that have pending messages in the next superstep. This ensures
	// parallel nodes completing at different times don't prevent message delivery.

	if len(candidates) == 0 {
		return nil, nil
	}

	next := make([]string, 0, len(candidates))
	for candidate := range candidates {
		// Skip END node - it's a sentinel, not an actual vertex
		if candidate == EndNode {
			continue
		}
		next = append(next, candidate)
	}
	sort.Strings(next)
	return next, nil
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

	for _, name := range s.cg.NodeNames() {
		snapshot[name] = SchedulerState{
			TopologyReady: readySet[name],
			GateOpen:      s.evaluator.IsGateOpen(name),
			Executed:      s.tracker.WasExecuted(name),
			Paused:        s.tracker.IsPaused(name),
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
// This resets all three components: topology, evaluator, and tracker.
func (s *vertexScheduler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.topology.Reset()
	s.evaluator.Reset()
	s.tracker.Reset()

	// Mark START node as executed to activate its downstream vertices
	downstream := s.cg.Outgoing()[StartNode]
	if len(downstream) > 0 {
		s.topology.MarkExecuted(StartNode, downstream)
	}
}

// EnsureVertexExists reports whether a vertex is tracked by the scheduler.
func (s *vertexScheduler) EnsureVertexExists(name string) bool {
	for _, n := range s.cg.NodeNames() {
		if n == name {
			return true
		}
	}
	return false
}

// applyCompletionForBootstrap processes a completed vertex's downstream during bootstrap.
func (s *vertexScheduler) applyCompletionForBootstrap(ctx context.Context, name string) {
	targets := s.cg.Outgoing()[name]
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
