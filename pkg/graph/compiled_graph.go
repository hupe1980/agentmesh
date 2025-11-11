package graph

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sort"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// Compiled is an immutable, validated graph ready for execution.
// It contains the topology (nodes, edges, conditionals) and runtime execution state.
// Compiled is safe for concurrent use across multiple goroutines.
//
// Concurrency Model:
//   - Multiple concurrent Run() calls are allowed (each gets independent state)
//   - Run() serializes execution via invokeMu (one invocation at a time)
//   - Runtime state access is protected by runtimeMu (RWMutex for read-heavy workload)
//
// Mutex Usage & Lock Ordering:
//
//  1. invokeMu: Coarse-grained lock preventing concurrent Run calls
//     - Acquired: Start of Run(), released at end
//     - Purpose: Prevents state corruption from concurrent executions
//     - Never held while calling into Pregel runtime
//
//  2. runtimeMu (RWMutex): Fine-grained lock for runtime state pointer
//     - Acquired: When reading/writing runtime pointer
//     - Purpose: Protects runtime state during graph execution
//     - Lock ordering: Always acquire BEFORE calling runtime methods
//     - Read-heavy: Most operations use RLock()
//
// Deadlock Prevention:
//   - Never acquire invokeMu while holding runtimeMu
//   - Never call external callbacks while holding any mutex
//
// Key Methods:
//   - Run: Execute graph and return an iterator over execution events
//
// Created by Builder.Compile() after graph construction.
type Compiled struct {
	stateManager      StateManager
	executor          Executor // Pluggable execution strategy
	runtime           *executionState
	runtimeMu         sync.RWMutex // Protects runtime state pointer
	invokeMu          sync.Mutex   // Serializes Run calls
	nodes             map[string]*Node
	edges             []Edge
	conditionals      []ConditionalEdges
	incoming          map[string]int
	conditionalGate   map[string]bool
	outgoing          map[string][]string
	conditionalByFrom map[string][]ConditionalEdges
	nodeNames         []string
	startKey          string
	endKey            string
}

func (cg *Compiled) hasExecutable(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := cg.nodes[name]; ok {
		return true
	}
	return false
}

func (cg *Compiled) markCompleted(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.markCompleted(name)
	}
}

func (cg *Compiled) markPaused(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.markPaused(name)
	}
}

func (cg *Compiled) clearPaused(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.clearPaused(name)
	}
}

func (cg *Compiled) setCurrentSuperstep(step int64) {
	cg.runtimeMu.Lock()
	defer cg.runtimeMu.Unlock()
	cg.runtime = ensureExecutionState(cg.runtime)
	cg.runtime.setSuperstep(step)
}

// CurrentSuperstep returns the current execution superstep.
func (cg *Compiled) CurrentSuperstep() int64 {
	cg.runtimeMu.RLock()
	defer cg.runtimeMu.RUnlock()
	if cg.runtime == nil {
		return 0
	}
	return cg.runtime.currentSuperstep()
}

// State returns the current graph state (for testing and diagnostics).
// In v2.0, this returns the StateManager interface instead of *State.
func (cg *Compiled) State() StateManager {
	return cg.stateManager
}

// topology creates an ExecutorTopology snapshot for executors.
// This provides a clean abstraction of the graph structure without exposing
// internal Compiled fields.
func (cg *Compiled) topology() *ExecutorTopology {
	return &ExecutorTopology{
		Nodes:             cg.nodes,
		Edges:             cg.edges,
		Conditionals:      cg.conditionals,
		Incoming:          cg.incoming,
		ConditionalGate:   cg.conditionalGate,
		Outgoing:          cg.outgoing,
		ConditionalByFrom: cg.conditionalByFrom,
		NodeNames:         cg.nodeNames,
		StartKey:          cg.startKey,
		EndKey:            cg.endKey,
	}
}

// attachProvidersToContext attaches observability providers from options to context.
// This ensures providers are available to node RunFuncs via FromContext() helpers.
func (cg *Compiled) attachProvidersToContext(ctx context.Context, options runOptions) context.Context {
	// Attach logger if configured
	if options.logger != nil {
		ctx = logging.WithLogger(ctx, options.logger)
	}

	// Attach tracer if configured
	if options.tracer != nil {
		ctx = trace.WithProvider(ctx, options.tracer)
	}

	// Attach metrics provider if configured
	if options.metricsProvider != nil {
		ctx = metrics.WithProvider(ctx, options.metricsProvider)
	}

	return ctx
}

// createInstrumentation builds an Instrumentation from the configured providers.
// Returns nil if no providers configured (noop behavior).
func (cg *Compiled) createInstrumentation(options runOptions) *Instrumentation {
	// Only create instrumentation if at least one provider is configured
	if options.tracer == nil && options.metricsProvider == nil {
		return nil
	}

	return newInstrumentation(options.metricsProvider, options.tracer)
}

func (cg *Compiled) bootstrapScheduler(ctx context.Context, s *vertexScheduler) {
	cg.runtimeMu.Lock()
	cg.runtime = ensureExecutionState(cg.runtime)
	runtime := cg.runtime
	cg.runtimeMu.Unlock()

	completed := runtime.completedNames()
	paused := runtime.pausedNames()

	s.Reset()
	s.Bootstrap(ctx, completed, paused)
}

// Run executes the graph with the given initial messages and returns an iterator
// of execution events. This is the primary API for graph execution.
//
// If a custom Executor is configured, execution is delegated to it.
// Otherwise, uses the default Pregel BSP execution.
func (cg *Compiled) Run(ctx context.Context, messages []message.Message, optFns ...RunOption) iter.Seq2[Event, error] {
	cg.invokeMu.Lock()
	defer cg.invokeMu.Unlock()

	options := defaultRunOptions()
	for _, optFn := range optFns {
		optFn(&options)
	}

	// If custom executor is configured, delegate to it
	if cg.executor != nil {
		runOpts := &RunOptions{
			MaxIterations:  options.maxIterations,
			MaxConcurrency: options.maxConcurrency,
			RunID:          options.runID,
		}
		return cg.executor.Run(ctx, cg.topology(), cg.stateManager, messages, runOpts)
	}

	// Default: use built-in Pregel BSP execution
	return cg.runWithOptions(ctx, messages, options)
}

// ApplyState synchronously merges values and messages into the committed graph state.
// Intended for external systems (e.g., human-in-the-loop workflows) to inject
// updates between supersteps without bypassing the staged execution pipeline.
func (cg *Compiled) ApplyState(values map[string]any, messages []Event) {
	if cg == nil || cg.stateManager == nil {
		return
	}
	cg.stateManager.ApplyUpdates(values, messages)
}

// AsNode wraps this Compiled as a Node that can be embedded in another graph.
// This enables subgraph composition and modular workflow construction.
// The subgraph's state is synchronized with the parent state before execution.
func (cg *Compiled) AsNode(name string) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			// Sync parent state into subgraph state before execution
			parentValues := s.GetAll()
			if parentValues != nil {
				cg.ApplyState(parentValues, nil)
			}

			// Get parent messages to pass to subgraph
			parentMessages := ExtractMessages(s.EventsSnapshot())

			// Execute the subgraph with parent messages and get final event
			_, err := Last(cg.Run(ctx, parentMessages))
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Return the subgraph's final state as updates
			// Get all accumulated events from state and convert to messages
			updates := cg.State().GetAll()
			events := cg.State().EventsSnapshot()
			msgs := make([]message.Message, len(events))
			for i := range events {
				msgs[i] = events[i].Message
			}

			return &NodeResult{
				Updates:  updates,
				Messages: msgs,
			}, nil
		},
	}
}

// AsNodeWithStateMapping wraps this Compiled as a Node with custom state mapping.
// mapInput transforms parent state into subgraph input state.
// mapOutput transforms subgraph output state into parent updates.
func (cg *Compiled) AsNodeWithStateMapping(
	name string,
	mapInput func(StateReader) (map[string]any, []Event),
	mapOutput func(StateReader) (map[string]any, []Event),
) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			// Map parent state to subgraph input
			var inputValues map[string]any
			var inputMessages []Event
			var messagesToInvoke []message.Message

			if mapInput != nil {
				inputValues, inputMessages = mapInput(s)
				if inputValues != nil || len(inputMessages) > 0 {
					cg.ApplyState(inputValues, inputMessages)
				}
				// Extract messages for Invoke
				messagesToInvoke = ExtractMessages(inputMessages)
			} else {
				// Get parent messages
				messagesToInvoke = ExtractMessages(s.EventsSnapshot())
			}

			// Execute subgraph with messages and get final event
			_, err := Last(cg.Run(ctx, messagesToInvoke))
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Map subgraph output to parent updates
			var updates map[string]any
			var messages []message.Message
			if mapOutput != nil {
				var events []Event
				updates, events = mapOutput(cg.State())
				// Convert events to []message.Message
				messages = make([]message.Message, len(events))
				for i := range events {
					messages[i] = events[i].Message
				}
			} else {
				updates = cg.State().GetAll()
				// Get all accumulated events from state and convert to messages
				events := cg.State().EventsSnapshot()
				messages = make([]message.Message, len(events))
				for i := range events {
					messages[i] = events[i].Message
				}
			}

			return &NodeResult{
				Updates:  updates,
				Messages: messages,
			}, nil
		},
	}
}

// restoreFromCheckpoint loads and restores checkpoint if configured.
// Returns the initial superstep to resume from, or 0 if no checkpoint was loaded.
func (cg *Compiled) restoreFromCheckpoint(ctx context.Context, options *runOptions, instrumentation *Instrumentation) (int64, error) {
	if options.checkpointer == nil || options.runID == "" || !options.autoRestore {
		return 0, nil
	}

	logger := logging.FromContext(ctx)
	var chkpt *checkpoint.Checkpoint
	var err error

	if options.resume && options.resumeFrom > 0 {
		// Resume from specific superstep
		logger.Info("loading checkpoint at specific superstep",
			"run_id", options.runID,
			"superstep", options.resumeFrom)
		chkpt, err = options.checkpointer.LoadAtSuperstep(ctx, options.runID, options.resumeFrom)
	} else {
		// Resume from latest checkpoint
		logger.Info("loading latest checkpoint",
			"run_id", options.runID)
		chkpt, err = options.checkpointer.Load(ctx, options.runID)
	}

	if err != nil {
		logger.Error("failed to load checkpoint",
			"run_id", options.runID,
			"error", err)
		return 0, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if chkpt == nil {
		return 0, nil
	}

	logger.Info("restoring from checkpoint",
		"run_id", options.runID,
		"superstep", chkpt.Superstep,
		"version", chkpt.Version)

	// Trace checkpoint restore operation (if instrumentation configured)
	if instrumentation != nil {
		restoreCtx, restoreSpan := instrumentation.TraceCheckpoint(ctx, "restore", options.runID, chkpt.Superstep)
		err = cg.restoreCheckpoint(chkpt)
		restoreSpan.End(err)
		_ = restoreCtx // Context not used further
	} else {
		err = cg.restoreCheckpoint(chkpt)
	}

	if err != nil {
		logger.Error("failed to restore checkpoint",
			"run_id", options.runID,
			"superstep", chkpt.Superstep,
			"error", err)
		return 0, fmt.Errorf("failed to restore checkpoint: %w", err)
	}

	logger.Info("checkpoint restored successfully",
		"run_id", options.runID,
		"superstep", chkpt.Superstep,
		"version", chkpt.Version)

	return chkpt.Superstep, nil
}

// setupRun prepares the execution context, instrumentation, and state for a graph run.
// It handles context validation, message preparation, checkpoint restoration, and provider setup.
func (cg *Compiled) setupRun(ctx context.Context, messages []message.Message, options *runOptions) (context.Context, *Instrumentation, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("%w", ErrNilContext)
	}
	if options.maxConcurrency < 1 {
		options.maxConcurrency = 1
	}

	// Wrap input messages as Events for internal processing
	if len(messages) > 0 {
		events := make([]Event, len(messages))
		for i, msg := range messages {
			events[i] = *NewEvent(msg, options.runID, "__input__")
		}
		if cg.stateManager != nil {
			cg.stateManager.ApplyUpdates(nil, events)
		}
	}

	// Attach observability providers to context
	ctx = cg.attachProvidersToContext(ctx, *options)

	// Create instrumentation from providers
	instrumentation := cg.createInstrumentation(*options)

	// Attempt to restore from checkpoint
	initialSuperstep, err := cg.restoreFromCheckpoint(ctx, options, instrumentation)
	if err != nil {
		return nil, nil, err
	}
	if initialSuperstep > 0 {
		options.initialSuperstep = initialSuperstep
	}

	return ctx, instrumentation, nil
}

func (cg *Compiled) runWithOptions(ctx context.Context, messages []message.Message, options runOptions) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		runCtx, instrumentation, err := cg.setupRun(ctx, messages, &options)
		if err != nil {
			yield(Event{}, err)
			return
		}

		derivedCtx, cancel := context.WithCancel(runCtx)
		defer cancel()

		// Create runtime with yield function directly
		rt := newPregelRuntime(cg, cancel, options, yield, instrumentation)
		_ = rt.run(derivedCtx)

		// Don't emit deadline exceeded errors here - they're already wrapped and emitted
		// by the node adapter with the specific node name. Only emit unexpected context errors.
		if err := derivedCtx.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			rt.emitError(err)
		}
	}
}

// =============================================================================
// ConditionalEvaluator - Manages conditional edge routing
// =============================================================================

// ConditionalEvaluator manages conditional edge routing based on runtime state.
// It evaluates condition functions and activates target vertices.
type ConditionalEvaluator struct {
	mu            sync.RWMutex
	cg            *Compiled
	gatedVertices map[string]bool // vertices behind conditional edges
	openGates     map[string]bool // gates that have been opened
}

// NewConditionalEvaluator creates an evaluator for the given graph.
func NewConditionalEvaluator(cg *Compiled) *ConditionalEvaluator {
	gated := make(map[string]bool)
	for name := range cg.conditionalGate {
		if cg.conditionalGate[name] {
			gated[name] = true
		}
	}

	return &ConditionalEvaluator{
		cg:            cg,
		gatedVertices: gated,
		openGates:     make(map[string]bool),
	}
}

// EvaluateFrom evaluates all conditional edges originating from the given vertex.
// Returns the list of newly activated target vertices.
func (ce *ConditionalEvaluator) EvaluateFrom(ctx context.Context, source string) ([]string, error) {
	conditionals := ce.cg.conditionalByFrom[source]
	if len(conditionals) == 0 {
		return nil, nil
	}

	activated := make(map[string]struct{})
	for _, conditional := range conditionals {
		if conditional.Condition == nil {
			continue
		}

		selected := conditional.Condition(ctx, ce.cg.stateManager)
		for _, target := range selected {
			if target == "" {
				continue
			}
			// Check if this is a valid target
			validTarget := false
			for _, allowed := range conditional.Targets {
				if target == allowed {
					validTarget = true
					break
				}
			}
			// Only activate executable nodes (not END)
			if validTarget && ce.cg.hasExecutable(target) {
				activated[target] = struct{}{}
			}
		}
	}

	// Open gates and return activated vertices
	ce.mu.Lock()
	defer ce.mu.Unlock()

	result := make([]string, 0, len(activated))
	for target := range activated {
		if ce.gatedVertices[target] && !ce.openGates[target] {
			ce.openGates[target] = true
		}
		result = append(result, target)
	}

	sort.Strings(result)
	return result, nil
}

// IsGateOpen checks if a gated vertex has been activated.
func (ce *ConditionalEvaluator) IsGateOpen(vertex string) bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	if !ce.gatedVertices[vertex] {
		return true // not gated, always open
	}
	return ce.openGates[vertex]
}

// Reset clears all open gates.
func (ce *ConditionalEvaluator) Reset() {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	for k := range ce.openGates {
		delete(ce.openGates, k)
	}
}

// BootstrapOpenGates marks specific gates as open (for resume scenarios).
func (ce *ConditionalEvaluator) BootstrapOpenGates(vertices []string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	for _, v := range vertices {
		if ce.gatedVertices[v] {
			ce.openGates[v] = true
		}
	}
}
