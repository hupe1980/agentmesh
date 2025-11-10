package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

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
//   - Multiple concurrent Stream() calls are allowed (each gets independent state)
//   - Invoke() serializes execution via invokeMu (one invocation at a time)
//   - Runtime state access is protected by runtimeMu (RWMutex for read-heavy workload)
//
// Mutex Usage & Lock Ordering:
//
//  1. invokeMu: Coarse-grained lock preventing concurrent Invoke/Stream calls
//     - Acquired: Start of Invoke(), released at end
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
//   - Always release mutex before emitting stream events
//
// Key Methods:
//   - Invoke: Execute graph and return final messages
//   - Stream: Execute graph with real-time event streaming
//
// Created by Builder.Compile() after graph construction.
type Compiled struct {
	stateManager      StateManager
	runtime           *executionState
	runtimeMu         sync.RWMutex // Protects runtime state pointer
	invokeMu          sync.Mutex   // Serializes Invoke/Stream calls
	nodes             map[string]*Node
	edges             []Edge
	conditionals      []ConditionalEdges
	incoming          map[string]int
	conditionalGate   map[string]bool
	outgoing          map[string][]string
	conditionalByFrom map[string][]ConditionalEdges
	nodeNames         []string
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

// Invoke executes the graph synchronously and returns the final message events.
// Returns MessageEvent slice with execution metadata (node, timestamp, etc).
func (cg *Compiled) Invoke(ctx context.Context, messages []message.Message, optFns ...RunOption) ([]MessageEvent, error) {
	cg.invokeMu.Lock()
	defer cg.invokeMu.Unlock()

	options := defaultRunOptions()
	for _, optFn := range optFns {
		optFn(&options)
	}

	return cg.invokeWithOptions(ctx, messages, options)
}

// Stream executes the graph and streams intermediate results.
func (cg *Compiled) Stream(ctx context.Context, messages []message.Message, optFns ...RunOption) (*Stream, error) {
	cg.invokeMu.Lock()
	defer cg.invokeMu.Unlock()

	options := defaultRunOptions()
	for _, optFn := range optFns {
		optFn(&options)
	}
	return cg.streamWithOptions(ctx, messages, options)
}

// ApplyState synchronously merges values and messages into the committed graph state.
// Intended for external systems (e.g., human-in-the-loop workflows) to inject
// updates between supersteps without bypassing the staged execution pipeline.
func (cg *Compiled) ApplyState(values map[string]any, messages []MessageEvent) {
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
			parentMessages := ExtractMessages(s.MessageEventsSnapshot())

			// Execute the subgraph with parent messages and get events with metadata
			events, err := cg.Invoke(ctx, parentMessages)
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Return the subgraph's final state as updates
			// Convert events to []message.Message (MessageEvent implements message.Message)
			updates := cg.State().GetAll()
			msgs := make([]message.Message, len(events))
			for i, evt := range events {
				msgs[i] = &evt
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
	mapInput func(StateReader) (map[string]any, []MessageEvent),
	mapOutput func(StateReader) (map[string]any, []MessageEvent),
) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			// Map parent state to subgraph input
			var inputValues map[string]any
			var inputMessages []MessageEvent
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
				messagesToInvoke = ExtractMessages(s.MessageEventsSnapshot())
			}

			// Execute subgraph with messages and get events with metadata
			events, err := cg.Invoke(ctx, messagesToInvoke)
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Map subgraph output to parent updates
			var updates map[string]any
			var messages []message.Message
			if mapOutput != nil {
				var messageEvents []MessageEvent
				updates, messageEvents = mapOutput(cg.State())
				// Convert events to []message.Message
				messages = make([]message.Message, len(messageEvents))
				for i, evt := range messageEvents {
					messages[i] = &evt
				}
			} else {
				updates = cg.State().GetAll()
				// Convert events to []message.Message (MessageEvent implements message.Message)
				messages = make([]message.Message, len(events))
				for i, evt := range events {
					messages[i] = &evt
				}
			}

			return &NodeResult{
				Updates:  updates,
				Messages: messages,
			}, nil
		},
	}
}

func (cg *Compiled) invokeWithOptions(ctx context.Context, messages []message.Message, options runOptions) ([]MessageEvent, error) {
	// Wrap input messages as MessageEvents for internal processing
	var messageEvents []MessageEvent
	if len(messages) > 0 {
		messageEvents = make([]MessageEvent, len(messages))
		for i, msg := range messages {
			messageEvents[i] = *NewMessageEvent(msg, options.runID, "__input__")
		}
	}

	// Attach observability providers to context if configured
	ctx = cg.attachProvidersToContext(ctx, options)

	// Create instrumentation from providers
	instrumentation := cg.createInstrumentation(options)

	// Start graph execution trace span
	var span trace.Span
	if instrumentation != nil {
		ctx, span = instrumentation.TraceGraphExecution(ctx, "graph.invoke")
		defer func() {
			if span != nil {
				span.End(nil)
			}
		}()
	}

	startTime := time.Now()
	stream, err := cg.streamWithOptions(ctx, messages, options)
	if err != nil {
		if span != nil {
			span.End(err)
		}
		return nil, err
	}
	defer stream.Cancel()

	for stream.Next() {
		event := stream.Current()
		if event.Err != nil {
			if span != nil {
				span.End(event.Err)
			}
			return nil, event.Err
		}
	}

	if err := stream.Err(); err != nil {
		if span != nil {
			span.End(err)
		}
		return nil, err
	}

	// Record metrics
	if instrumentation != nil {
		instrumentation.RecordGraphExecution(ctx, "graph.invoke", time.Since(startTime), true)
	}

	if cg == nil || cg.stateManager == nil {
		return nil, nil
	}

	// Return message events with execution metadata
	return cg.stateManager.MessageEventsSnapshot(), nil
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

func (cg *Compiled) streamWithOptions(ctx context.Context, messages []message.Message, options runOptions) (*Stream, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w", ErrNilContext)
	}
	if options.maxConcurrency < 1 {
		options.maxConcurrency = 1
	}

	// Wrap input messages as MessageEvents for internal processing
	var messageEvents []MessageEvent
	if len(messages) > 0 {
		messageEvents = make([]MessageEvent, len(messages))
		for i, msg := range messages {
			messageEvents[i] = *NewMessageEvent(msg, options.runID, "__input__")
		}
	}

	// Attach observability providers to context if configured
	ctx = cg.attachProvidersToContext(ctx, options)

	// Create instrumentation from providers
	instrumentation := cg.createInstrumentation(options)

	// Attempt to restore from checkpoint if configured
	initialSuperstep, err := cg.restoreFromCheckpoint(ctx, &options, instrumentation)
	if err != nil {
		return nil, err
	}
	if initialSuperstep > 0 {
		options.initialSuperstep = initialSuperstep
	}

	if len(messageEvents) > 0 && cg != nil && cg.stateManager != nil {
		cg.stateManager.ApplyUpdates(nil, messageEvents)
	}

	derivedCtx, cancel := context.WithCancel(ctx)
	// Use configurable event buffer size
	bufferSize := options.eventBufferSize
	if bufferSize <= 0 {
		bufferSize = 100 // Fallback to default
	}
	events := make(chan StreamEvent, bufferSize) // Buffered to reduce blocking
	done := make(chan struct{})                  // Signal for early termination

	go func() {
		defer close(events)
		defer close(done)
		defer cancel()

		rt := newPregelRuntime(cg, cancel, options, events, done, instrumentation)
		_ = rt.run(derivedCtx) // Pass context to run() method

		// Don't emit deadline exceeded errors here - they're already wrapped and emitted
		// by the node adapter with the specific node name. Only emit unexpected context errors.
		if err := derivedCtx.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			rt.emitError(err)
		}
	}()

	return newStream(events, cancel, done), nil
}

// StreamEvent represents a single event emitted during graph execution.
// Events are emitted after each node completes execution.
type StreamEvent struct {
	Node     string         // Name of the node that completed
	Updates  map[string]any // State updates from the node
	Messages []MessageEvent // New message events appended by the node
	Result   *NodeResult    // Full node result (Updates + Messages)
	Err      error          // Error if node execution failed
}

// Stream provides an iterator over graph execution events.
// Use Next() to advance and Event() to retrieve the current event.
// IMPORTANT: Always call Cancel() or Close() when done to prevent goroutine leaks.
type Stream struct {
	events  <-chan StreamEvent
	cancel  context.CancelFunc
	done    <-chan struct{} // Signals when background goroutine completes
	current StreamEvent
	err     error
	closed  bool
	mu      sync.Mutex
}

func newStream(events <-chan StreamEvent, cancel context.CancelFunc, done <-chan struct{}) *Stream {
	return &Stream{
		events: events,
		cancel: cancel,
		done:   done,
	}
}

// Next advances to the next stream event.
func (s *Stream) Next() bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()

	event, ok := <-s.events
	if !ok {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		return false
	}

	s.mu.Lock()
	s.current = event
	if event.Err != nil {
		if s.err == nil {
			s.err = event.Err
		}
		s.closed = true
	}
	s.mu.Unlock()

	return true
}

// Current returns the current stream event.
func (s *Stream) Current() StreamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Err returns any error encountered during streaming.
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Cancel stops the stream and releases resources.
func (s *Stream) Cancel() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
}

// Close cancels the stream and waits for the background goroutine to finish.
// This prevents goroutine leaks when the consumer stops reading events early.
// Close is idempotent and safe to call multiple times.
func (s *Stream) Close() error {
	s.Cancel()
	// Wait for background goroutine to exit
	if s.done != nil {
		<-s.done
	}
	return nil
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
