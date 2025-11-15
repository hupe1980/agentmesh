package graph

import (
	"context"
	"errors"
	"iter"
	"runtime"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// SchedulingMessage describes an activation message between graph vertices.
// Used in distributed scheduling to propagate computation across nodes.
type SchedulingMessage struct {
	From string
	To   string
}

// Combiner merges multiple scheduling messages for the same target.
// Used to optimize message passing by reducing redundant activations.
type Combiner func(existing, incoming SchedulingMessage) SchedulingMessage

// PregelExecutor executes graphs using the Pregel Bulk Synchronous Parallel (BSP) engine.
// This is the default high-performance executor with support for:
//   - Parallel execution via worker pools
//   - Distributed execution via pluggable MessageBus backends
//   - Aggregators for global state reduction across supersteps
//   - Message combiners for optimization
//
// PregelExecutor is configured with PregelOption functions at construction time.
// Once created, configuration is immutable and has no dependencies on specific graph instances.
//
// Example:
//
//	executor := graph.NewPregelExecutor(
//	    graph.WithMessageBus(redisBus),
//	    graph.WithPregelAggregators(map[string]pregel.Aggregator{
//	        "total_cost": &graph.SumAggregator{},
//	    }),
//	    graph.WithMaxWorkers(16),
//	)
//	g.WithExecutor(executor)
type PregelExecutor struct {
	messageBus  pregel.MessageBus[ChannelMessage]
	aggregators map[string]pregel.Aggregator
	combiner    Combiner
	maxWorkers  int
	maxIters    int
	runtime     *executionState // Runtime state for pause/resume and superstep tracking
}

// PregelOption configures a PregelExecutor.
type PregelOption func(*PregelExecutor)

// NewPregelExecutor creates a new Pregel BSP executor with the given options.
// This is the default high-performance executor for AgentMesh graphs.
//
// Default configuration:
//   - MaxWorkers: runtime.NumCPU()
//   - MaxIterations: unlimited (0)
//   - MessageBus: in-memory (created automatically)
//   - Aggregators: none
//   - Combiner: none
//
// Example:
//
//	executor := graph.NewPregelExecutor(
//	    graph.WithMaxWorkers(16),
//	    graph.WithMessageBus(redisBus),
//	)
func NewPregelExecutor(opts ...PregelOption) *PregelExecutor {
	pe := &PregelExecutor{
		maxWorkers: runtime.NumCPU(),
		maxIters:   0,                   // unlimited
		runtime:    newExecutionState(), // Initialize runtime state for pause/resume
	}
	for _, opt := range opts {
		opt(pe)
	}
	return pe
}

// WithMessageBus sets a custom message bus for distributed graph execution.
// This enables multi-process or multi-node execution using a shared message
// delivery backend (e.g., Redis, Kafka).
//
// If not provided, an in-memory message bus is used (single-process only).
//
// Example:
//
//	import predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
//
//	bus := predis.NewMessageBus[graph.ChannelMessage]("localhost:6379", "", 0, &predis.Options{
//	    Namespace: "my-graph",
//	    TTL: 1 * time.Hour,
//	})
//	defer bus.Close()
//
//	executor := graph.NewPregelExecutor(graph.WithMessageBus(bus))
func WithMessageBus(bus pregel.MessageBus[ChannelMessage]) PregelOption {
	return func(pe *PregelExecutor) {
		pe.messageBus = bus
	}
}

// WithPregelAggregators configures global aggregators for distributed reductions.
// Aggregators collect values from all nodes in each superstep and make the
// result available in the next superstep via state.AggregatesSnapshot().
//
// Common use cases:
//   - Sum: Total cost, count of nodes processed
//   - Max/Min: Highest priority, earliest timestamp
//   - Custom: Complex reductions (e.g., histogram, average)
//
// Example:
//
//	executor := graph.NewPregelExecutor(
//	    graph.WithPregelAggregators(map[string]pregel.Aggregator{
//	        "total_cost": &graph.SumAggregator{},
//	        "max_priority": &graph.MaxAggregator{},
//	    }),
//	)
func WithPregelAggregators(aggregators map[string]pregel.Aggregator) PregelOption {
	return func(pe *PregelExecutor) {
		if len(aggregators) == 0 {
			pe.aggregators = nil
			return
		}
		aggCopy := make(map[string]pregel.Aggregator, len(aggregators))
		for name, agg := range aggregators {
			if name == "" || agg == nil {
				continue
			}
			aggCopy[name] = agg
		}
		pe.aggregators = aggCopy
	}
}

// WithPregelCombiner sets a function to merge multiple messages targeting the same node.
// This optimization can reduce redundant activations in highly connected graphs.
//
// The combiner receives existing and incoming messages and returns the merged result.
// This is called during message delivery to reduce the number of activations.
//
// Example:
//
//	executor := graph.NewPregelExecutor(
//	    graph.WithPregelCombiner(func(existing, incoming graph.SchedulingMessage) graph.SchedulingMessage {
//	        // Simple: last message wins
//	        return incoming
//	    }),
//	)
func WithPregelCombiner(combiner Combiner) PregelOption {
	return func(pe *PregelExecutor) {
		pe.combiner = combiner
	}
}

// WithMaxWorkers sets the maximum number of worker goroutines for parallel execution.
// More workers can improve throughput for I/O-bound nodes but increase memory usage.
//
// Default: runtime.NumCPU()
// Recommended: 2-4x CPU count for I/O-bound workloads, CPU count for CPU-bound
//
// Example:
//
//	executor := graph.NewPregelExecutor(graph.WithMaxWorkers(16))
func WithMaxWorkers(n int) PregelOption {
	return func(pe *PregelExecutor) {
		if n > 0 {
			pe.maxWorkers = n
		}
	}
}

// WithPregelMaxIterations sets the maximum number of supersteps before terminating execution.
// This prevents infinite loops in cyclic graphs.
//
// Default: 0 (unlimited)
// Recommended: Set a reasonable limit for production (e.g., 100-1000) to prevent runaway loops
//
// Example:
//
//	executor := graph.NewPregelExecutor(graph.WithPregelMaxIterations(1000))
func WithPregelMaxIterations(n int) PregelOption {
	return func(pe *PregelExecutor) {
		pe.maxIters = n
	}
}

// =============================================================================
// Executor Interface Implementation
// =============================================================================

// Run executes the graph using Pregel BSP execution.
// This implements the Executor interface and performs the actual graph execution.
//
// The execution flow:
//  1. Build internal runOptions from RunOptions and PregelExecutor config
//  2. Setup run context with instrumentation and checkpoint restoration
//  3. Create and run the Pregel runtime
//  4. Emit execution events via the iterator
//
// This method is completely self-contained and has no dependencies on Compiled.
// All necessary context is provided through the Executor interface parameters.
func (pe *PregelExecutor) Run(
	ctx context.Context,
	topology *ExecutorTopology,
	stateManager StateManager,
	initialMessages []message.Message,
	options *RunOptions,
) iter.Seq2[state.ExecutionResult, error] {
	runOpts := pe.buildRunOptions(options)

	// Execute using only the provided parameters - no dependency on Compiled
	return func(yield func(state.ExecutionResult, error) bool) {
		// Setup execution context with instrumentation and checkpoint restoration
		runCtx, instrumentation, resume, err := setupRun(ctx, stateManager, initialMessages, &runOpts)
		if err != nil {
			yield(state.ExecutionResult{}, err)
			return
		}

		derivedCtx, cancel := context.WithCancel(runCtx)
		defer cancel()

		// Create execution runtime with checkpoint restoration and paused nodes
		executionRuntime := pe.createExecutionRuntime(resume)

		structure := &topologyAdapter{
			topology:     topology,
			stateManager: stateManager,
			runtime:      executionRuntime,
		}

		// Create runtime with yield function directly
		rt := newPregelRuntime(structure, cancel, runOpts, yield, instrumentation)
		_ = rt.run(derivedCtx)

		// Sync execution runtime state back to executor's persistent runtime
		pe.syncRuntimeState(executionRuntime)

		// Don't emit deadline exceeded errors here - they're already wrapped and emitted
		// by the node adapter with the specific node name. Only emit unexpected context errors.
		if err := derivedCtx.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			rt.emitError(err)
		}
	}
}

// buildRunOptions constructs runOptions by merging RunOptions with PregelExecutor configuration.
func (pe *PregelExecutor) buildRunOptions(options *RunOptions) runOptions {
	// Get internal runOptions from RunOptions if provided, otherwise create defaults
	var runOpts runOptions
	if options.internal != nil {
		runOpts = *options.internal
	} else {
		runOpts = defaultRunOptions()
		runOpts.runID = options.RunID
		runOpts.maxIterations = options.MaxIterations
		runOpts.maxConcurrency = options.MaxConcurrency
	}

	// Merge PregelExecutor configuration into runOptions
	// PregelExecutor config takes precedence over RunOptions
	if pe.maxWorkers > 0 {
		runOpts.maxConcurrency = pe.maxWorkers
	}
	if pe.maxIters > 0 {
		runOpts.maxIterations = pe.maxIters
	}
	if len(pe.aggregators) > 0 {
		if runOpts.aggregators == nil {
			runOpts.aggregators = make(map[string]interface{})
		}
		for k, v := range pe.aggregators {
			runOpts.aggregators[k] = v
		}
	}
	if pe.combiner != nil {
		runOpts.combiner = pe.combiner
	}
	if pe.messageBus != nil {
		runOpts.messageBus = pe.messageBus
	}

	return runOpts
}

// createExecutionRuntime creates a new execution runtime, restoring checkpoint state
// and copying paused nodes from the executor's persistent runtime.
func (pe *PregelExecutor) createExecutionRuntime(resume *checkpointResume) *executionState {
	executionRuntime := newExecutionState()

	// If resuming from checkpoint, restore completed and paused nodes
	if resume != nil {
		executionRuntime.mu.Lock()
		for _, nodeName := range resume.completedNodes {
			executionRuntime.completed[nodeName] = true
		}
		for _, nodeName := range resume.pausedNodes {
			executionRuntime.paused[nodeName] = true
		}
		executionRuntime.mu.Unlock()
	}

	// Copy paused nodes from executor's persistent runtime to execution runtime
	// (these override checkpoint paused nodes if both exist)
	if pe.runtime != nil {
		pe.runtime.mu.Lock()
		// Make a copy of the map while holding the lock to avoid race conditions
		pausedCopy := make(map[string]bool, len(pe.runtime.paused))
		for nodeName := range pe.runtime.paused {
			pausedCopy[nodeName] = true
		}
		pe.runtime.mu.Unlock()

		// Now copy to executionRuntime without holding pe.runtime lock
		executionRuntime.mu.Lock()
		for nodeName := range pausedCopy {
			executionRuntime.paused[nodeName] = true
		}
		executionRuntime.mu.Unlock()
	}

	return executionRuntime
}

// syncRuntimeState synchronizes the superstep count from execution runtime back to
// the executor's persistent runtime. This ensures CurrentSuperstep() returns the
// correct value after execution completes.
func (pe *PregelExecutor) syncRuntimeState(executionRuntime *executionState) {
	if pe.runtime != nil && executionRuntime != nil {
		pe.runtime.mu.Lock()
		pe.runtime.setSuperstep(executionRuntime.currentSuperstep())
		pe.runtime.mu.Unlock()
	}
}

// CurrentSuperstep returns the current superstep number.
// For PregelExecutor, this is managed by the underlying Pregel runtime.
func (pe *PregelExecutor) CurrentSuperstep() int64 {
	if pe.runtime == nil {
		return 0
	}
	return pe.runtime.currentSuperstep()
}

// Pause marks a node to pause before its next execution.
// The node will be skipped during graph execution until Resume is called.
// This is for external control and persists across Run() calls.
func (pe *PregelExecutor) Pause(nodeName string) {
	if pe.runtime != nil {
		pe.runtime.markPaused(nodeName)
	}
}

// Resume clears the pause state for a node.
// The node will be executed normally in subsequent executions.
// This is for external control and persists across Run() calls.
func (pe *PregelExecutor) Resume(nodeName string) {
	if pe.runtime != nil {
		pe.runtime.clearPaused(nodeName)
	}
}

// IsPaused checks if a node is currently paused.
// Returns true if the node was marked paused via Pause() and not yet resumed.
func (pe *PregelExecutor) IsPaused(nodeName string) bool {
	if pe.runtime == nil {
		return false
	}
	pe.runtime.mu.Lock()
	defer pe.runtime.mu.Unlock()
	return pe.runtime.paused[nodeName]
}

// Verify PregelExecutor implements Executor interface at compile time
var _ Executor = (*PregelExecutor)(nil)

// =============================================================================
// Topology Adapter - Adapts ExecutorTopology to Structure interface
// =============================================================================

// topologyAdapter adapts ExecutorTopology + StateManager to the Structure interface
// required by newPregelRuntime. This adapter is created per execution and has no
// dependencies on Compiled, maintaining clean architecture.
type topologyAdapter struct {
	topology     *ExecutorTopology
	stateManager StateManager
	runtime      *executionState
}

func (ta *topologyAdapter) Nodes() map[string]*Node {
	return ta.topology.Nodes
}

func (ta *topologyAdapter) Outgoing() map[string][]string {
	return ta.topology.Outgoing
}

func (ta *topologyAdapter) Incoming() map[string]int {
	return ta.topology.Incoming
}

func (ta *topologyAdapter) ConditionalByFrom() map[string][]ConditionalEdges {
	return ta.topology.ConditionalByFrom
}

func (ta *topologyAdapter) ConditionalGate() map[string]bool {
	return ta.topology.ConditionalGate
}

func (ta *topologyAdapter) NodeNames() []string {
	return ta.topology.NodeNames
}

func (ta *topologyAdapter) StartKey() string {
	return ta.topology.StartKey
}

func (ta *topologyAdapter) EndKey() string {
	return ta.topology.EndKey
}

func (ta *topologyAdapter) StateManager() StateManager {
	return ta.stateManager
}

func (ta *topologyAdapter) HasExecutable(name string) bool {
	if name == "" {
		return false
	}
	_, ok := ta.topology.Nodes[name]
	return ok
}

func (ta *topologyAdapter) MarkCompleted(name string) {
	if ta.runtime != nil {
		ta.runtime.markCompleted(name)
	}
}

func (ta *topologyAdapter) MarkPaused(name string) {
	if ta.runtime != nil {
		ta.runtime.markPaused(name)
	}
}

func (ta *topologyAdapter) ClearPaused(name string) {
	if ta.runtime != nil {
		ta.runtime.clearPaused(name)
	}
}

func (ta *topologyAdapter) SetCurrentSuperstep(step int64) {
	if ta.runtime != nil {
		ta.runtime.setSuperstep(step)
	}
}

func (ta *topologyAdapter) CreateCheckpoint(runID string, superstep int64, metadata map[string]any) *checkpoint.Checkpoint {
	// Build checkpoint from current state
	var completedNodes, pausedNodes []string
	if ta.runtime != nil {
		completedNodes = ta.runtime.completedNames()
		pausedNodes = ta.runtime.pausedNames()
	}

	return &checkpoint.Checkpoint{
		RunID:          runID,
		Superstep:      superstep,
		State:          ta.stateManager.GetAll(),
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
		Metadata:       metadata,
		Version:        1,
	}
}

func (ta *topologyAdapter) BootstrapScheduler(ctx context.Context, s *vertexScheduler) {
	// Pass completed/paused nodes from executionRuntime to scheduler
	var completed, paused []string
	if ta.runtime != nil {
		completed = ta.runtime.completedNames()
		paused = ta.runtime.pausedNames()
	}
	s.Bootstrap(ctx, completed, paused)
}
