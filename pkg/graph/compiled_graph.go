package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	streamutil "github.com/hupe1980/agentmesh/internal/stream"
	"github.com/hupe1980/agentmesh/pkg/message"
	"golang.org/x/time/rate"
)

// CompiledGraph is an immutable, validated graph ready for execution.
// It contains the topology (nodes, edges, conditionals) and runtime execution state.
// CompiledGraph is safe for concurrent use across multiple goroutines.
//
// Concurrency Model:
//   - Multiple concurrent Stream() calls are allowed (each gets independent state)
//   - Invoke() serializes execution via invokeMu (one invocation at a time)
//   - Runtime state access is protected by runtimeMu (RWMutex for read-heavy workload)
//   - Rate limiters are protected by rateLimitersMu (RWMutex for read-heavy workload)
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
//  3. rateLimitersMu (RWMutex): Fine-grained lock for rate limiter map
//     - Acquired: When accessing per-node rate limiters
//     - Purpose: Protects concurrent rate limiter access
//     - Lock ordering: Independent, can be acquired in any order
//     - Read-heavy: Most operations use RLock()
//
// Deadlock Prevention:
//   - Never acquire invokeMu while holding runtimeMu or rateLimitersMu
//   - Never call external callbacks while holding any mutex
//   - Always release mutex before emitting stream events
//
// Key Methods:
//   - Invoke: Execute graph and return final messages
//   - Stream: Execute graph with real-time event streaming
//
// Created by Builder.Compile() after graph construction.
type CompiledGraph struct {
	stateManager      StateManager
	runtime           *executionState
	runtimeMu         sync.RWMutex             // Protects runtime state pointer (see concurrency model above)
	invokeMu          sync.Mutex               // Serializes Invoke/Stream calls (see concurrency model above)
	rateLimitersMu    sync.RWMutex             // Protects rate limiter map (see concurrency model above)
	rateLimiters      map[string]*rate.Limiter // Per-node rate limiters
	nodes             map[string]*Node
	edges             []Edge
	conditionals      []ConditionalEdges
	incoming          map[string]int
	conditionalGate   map[string]bool
	outgoing          map[string][]string
	conditionalByFrom map[string][]ConditionalEdges
	nodeNames         []string
}

func (cg *CompiledGraph) hasExecutable(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := cg.nodes[name]; ok {
		return true
	}
	return false
}

func (cg *CompiledGraph) markCompleted(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.markCompleted(name)
	}
}

func (cg *CompiledGraph) markPaused(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.markPaused(name)
	}
}

func (cg *CompiledGraph) clearPaused(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.clearPaused(name)
	}
}

func (cg *CompiledGraph) setCurrentSuperstep(step int64) {
	cg.runtimeMu.Lock()
	defer cg.runtimeMu.Unlock()
	cg.runtime = ensureExecutionState(cg.runtime)
	cg.runtime.setSuperstep(step)
}

func (cg *CompiledGraph) CurrentSuperstep() int64 {
	cg.runtimeMu.RLock()
	defer cg.runtimeMu.RUnlock()
	if cg.runtime == nil {
		return 0
	}
	return cg.runtime.currentSuperstep()
}

// State returns the current graph state (for testing and diagnostics).
// In v2.0, this returns the StateManager interface instead of *GraphState.
func (cg *CompiledGraph) State() StateManager {
	return cg.stateManager
}

func (cg *CompiledGraph) bootstrapScheduler(ctx context.Context, s *vertexScheduler) {
	cg.runtimeMu.Lock()
	cg.runtime = ensureExecutionState(cg.runtime)
	runtime := cg.runtime
	cg.runtimeMu.Unlock()

	completed := runtime.completedNames()
	paused := runtime.pausedNames()

	s.Reset()
	s.Bootstrap(ctx, completed, paused)
}

func (cg *CompiledGraph) Invoke(ctx context.Context, messages []message.Message, optFns ...RunOption) ([]message.Message, error) {
	cg.invokeMu.Lock()
	defer cg.invokeMu.Unlock()

	options := defaultRunOptions()
	for _, optFn := range optFns {
		optFn(&options)
	}

	// Merge rate limiters into graph (persists across invocations)
	if len(options.rateLimiters) > 0 {
		cg.rateLimitersMu.Lock()
		if cg.rateLimiters == nil {
			cg.rateLimiters = make(map[string]*rate.Limiter)
		}
		for nodeName, limiter := range options.rateLimiters {
			if _, exists := cg.rateLimiters[nodeName]; !exists {
				cg.rateLimiters[nodeName] = limiter
			}
		}
		cg.rateLimitersMu.Unlock()
	}

	return cg.invokeWithOptions(ctx, messages, options)
}

func (cg *CompiledGraph) Stream(ctx context.Context, messages []message.Message, optFns ...RunOption) (*GraphStream, error) {
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
func (cg *CompiledGraph) ApplyState(values map[string]any, messages []message.Message) {
	if cg == nil || cg.stateManager == nil {
		return
	}
	cg.stateManager.ApplyUpdates(values, messages)
}

// AsNode wraps this CompiledGraph as a Node that can be embedded in another graph.
// This enables subgraph composition and modular workflow construction.
// The subgraph's state is synchronized with the parent state before execution.
func (cg *CompiledGraph) AsNode(name string) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			// Sync parent state into subgraph state before execution
			parentValues := s.GetAll()
			if parentValues != nil {
				cg.ApplyState(parentValues, nil)
			}

			// Execute the subgraph
			_, err := cg.Invoke(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Return the subgraph's final state as updates
			updates := cg.State().GetAll()
			msgs := cg.State().MessagesSnapshot()

			return &NodeResult{
				Updates:  updates,
				Messages: msgs,
			}, nil
		},
	}
}

// AsNodeWithStateMapping wraps this CompiledGraph as a Node with custom state mapping.
// mapInput transforms parent state into subgraph input state.
// mapOutput transforms subgraph output state into parent updates.
func (cg *CompiledGraph) AsNodeWithStateMapping(
	name string,
	mapInput func(StateReader) (map[string]any, []message.Message),
	mapOutput func(StateReader) (map[string]any, []message.Message),
) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			// Map parent state to subgraph input
			var inputValues map[string]any
			var inputMessages []message.Message
			if mapInput != nil {
				inputValues, inputMessages = mapInput(s)
				if inputValues != nil || len(inputMessages) > 0 {
					cg.ApplyState(inputValues, inputMessages)
				}
			}

			// Execute subgraph
			_, err := cg.Invoke(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Map subgraph output to parent updates
			var updates map[string]any
			var messages []message.Message
			if mapOutput != nil {
				updates, messages = mapOutput(cg.State())
			} else {
				updates = cg.State().GetAll()
				messages = cg.State().MessagesSnapshot()
			}

			return &NodeResult{
				Updates:  updates,
				Messages: messages,
			}, nil
		},
	}
}

func (cg *CompiledGraph) invokeWithOptions(ctx context.Context, messages []message.Message, options runOptions) ([]message.Message, error) {
	stream, err := cg.streamWithOptions(ctx, messages, options)
	if err != nil {
		return nil, err
	}
	defer stream.Cancel()
	for stream.Next() {
		event := stream.Current()
		if event.Err != nil {
			return nil, event.Err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if cg == nil || cg.stateManager == nil {
		return nil, nil
	}
	return cg.stateManager.MessagesSnapshot(), nil
}

//nolint:gocyclo // Streaming requires handling many configuration options and error cases
func (cg *CompiledGraph) streamWithOptions(ctx context.Context, messages []message.Message, options runOptions) (*GraphStream, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w", ErrNilContext)
	}
	if options.maxConcurrency < 1 {
		options.maxConcurrency = 1
	}

	// Attempt to restore from checkpoint if configured
	if options.checkpointer != nil && options.runID != "" && options.autoRestore {
		var checkpoint *Checkpoint
		var err error

		if options.resume && options.resumeFrom > 0 {
			// Resume from specific superstep
			checkpoint, err = options.checkpointer.LoadAtSuperstep(ctx, options.runID, options.resumeFrom)
		} else {
			// Resume from latest checkpoint
			checkpoint, err = options.checkpointer.Load(ctx, options.runID)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to load checkpoint: %w", err)
		}

		if checkpoint != nil {
			if err := cg.restoreCheckpoint(checkpoint); err != nil {
				return nil, fmt.Errorf("failed to restore checkpoint: %w", err)
			}
			// Set initial superstep to resume from
			options.initialSuperstep = checkpoint.Superstep
		}
	}

	if len(messages) > 0 && cg != nil && cg.stateManager != nil {
		cg.stateManager.ApplyUpdates(nil, messages)
	}

	derivedCtx, cancel := context.WithCancel(ctx)
	events := make(chan StreamEvent, 100) // Buffered to reduce blocking
	done := make(chan struct{})           // Signal for early termination

	go func() {
		defer close(events)
		defer close(done)
		defer cancel()

		rt := newPregelRuntime(cg, derivedCtx, cancel, options, events, done)
		_ = rt.run() // Errors are emitted as events

		// Don't emit deadline exceeded errors here - they're already wrapped and emitted
		// by the node adapter with the specific node name. Only emit unexpected context errors.
		if err := derivedCtx.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			rt.emitError(err)
		}
	}()

	return newGraphStream(events, cancel, done), nil
}

// StreamEvent represents a single event emitted during graph execution.
// Events are emitted after each node completes execution.
type StreamEvent struct {
	Node     string            // Name of the node that completed
	Updates  map[string]any    // State updates from the node
	Messages []message.Message // New messages appended by the node
	Result   *NodeResult       // Full node result (Updates + Messages)
	Err      error             // Error if node execution failed
}

// GraphStream provides an iterator over graph execution events.
// Use Next() to advance and Event() to retrieve the current event.
// IMPORTANT: Always call Cancel() or Close() when done to prevent goroutine leaks.
type GraphStream struct {
	inner *streamutil.Stream[StreamEvent]
	done  <-chan struct{} // Signals when background goroutine completes
}

func newGraphStream(events <-chan StreamEvent, cancel context.CancelFunc, done <-chan struct{}) *GraphStream {
	cfg := streamutil.Config[StreamEvent]{
		ExtractErr: func(event StreamEvent) error { return event.Err },
		StopOnErr:  true,
	}
	return &GraphStream{
		inner: streamutil.New(events, cancel, cfg),
		done:  done,
	}
}

func (s *GraphStream) Next() bool {
	if s == nil || s.inner == nil {
		return false
	}
	return s.inner.Next()
}

func (s *GraphStream) Current() StreamEvent {
	if s == nil || s.inner == nil {
		return StreamEvent{}
	}
	return s.inner.Current()
}

func (s *GraphStream) Err() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Err()
}

func (s *GraphStream) Cancel() {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.Cancel()
}

// Close cancels the stream and waits for the background goroutine to finish.
// This prevents goroutine leaks when the consumer stops reading events early.
// Close is idempotent and safe to call multiple times.
func (s *GraphStream) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
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
	cg            *CompiledGraph
	gatedVertices map[string]bool // vertices behind conditional edges
	openGates     map[string]bool // gates that have been opened
}

// NewConditionalEvaluator creates an evaluator for the given graph.
func NewConditionalEvaluator(cg *CompiledGraph) *ConditionalEvaluator {
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
