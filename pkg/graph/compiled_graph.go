package graph

import (
	"context"
	"fmt"
	"iter"
	"sort"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// Structure defines the interface for accessing graph structure.
// This interface decouples runtime components (graphRuntime, vertexScheduler, ConditionalEvaluator)
// from the concrete Compiled type, enabling better testability and extensibility.
//
// The interface provides read-only access to:
//   - Node registry (nodes by name)
//   - Edge topology (incoming/outgoing edges)
//   - Conditional routing (conditional edges and gates)
//   - Graph metadata (start/end nodes, node names)
//   - State management (access to StateManager)
//   - Execution tracking (mark completed/paused, superstep management)
type Structure interface {
	// Nodes returns the node with the given name, or nil if not found.
	Nodes() map[string]*Node

	// Outgoing returns the outgoing edges for the given node name.
	Outgoing() map[string][]string

	// Incoming returns the incoming edge count for each node.
	Incoming() map[string]int

	// ConditionalByFrom returns conditional edges grouped by source node.
	ConditionalByFrom() map[string][]ConditionalEdges

	// ConditionalGate returns whether a node is behind a conditional gate.
	ConditionalGate() map[string]bool

	// NodeNames returns the sorted list of all node names.
	NodeNames() []string

	// StartKey returns the start node name.
	StartKey() string

	// EndKey returns the end node name.
	EndKey() string

	// StateManager returns the graph's state manager.
	StateManager() StateManager

	// HasExecutable checks if a node with the given name exists.
	HasExecutable(name string) bool

	// MarkCompleted marks a node as completed.
	MarkCompleted(name string)

	// MarkPaused marks a node as paused.
	MarkPaused(name string)

	// ClearPaused clears the paused state for a node.
	ClearPaused(name string)

	// SetCurrentSuperstep sets the current execution superstep.
	SetCurrentSuperstep(step int64)

	// CreateCheckpoint creates a checkpoint snapshot.
	CreateCheckpoint(runID string, superstep int64, metadata map[string]any) *checkpoint.Checkpoint

	// BootstrapScheduler initializes the scheduler with persisted state.
	BootstrapScheduler(ctx context.Context, s *vertexScheduler)
}

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
// Compiled implements the MessageRunnable interface, enabling type-safe
// composition with other agents and graphs. All agent constructors
// (NewReActAgent, NewSupervisorAgent, NewRAGAgent) return MessageRunnable.
//
// Key Methods:
//   - Run: Execute graph and return an iterator over execution events
//
// Created by Builder.Compile() after graph construction.
type Compiled struct {
	stateManager      StateManager
	executor          Executor // Execution strategy (PregelExecutor, SimpleGraphExecutor, etc.)
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

// Compile-time check that Compiled implements MessageRunnable.
var _ MessageRunnable = (*Compiled)(nil)

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
func attachProvidersToContext(ctx context.Context, options runOptions) context.Context {
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
func createInstrumentation(options runOptions) *Instrumentation {
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

	// If resuming from checkpoint (has completed nodes), don't reset the scheduler.
	// The completed nodes define the frontier to resume from.
	// If starting fresh (no completed nodes), reset to start from START node.
	if len(completed) == 0 && len(paused) == 0 {
		s.Reset()
	}

	s.Bootstrap(ctx, completed, paused)
}

// Run executes the graph with the given initial messages and returns an iterator
// of execution events. This is the primary API for graph execution.
//
// EXECUTION STRATEGY:
//
// By default, Run uses the Pregel BSP (Bulk Synchronous Parallel) execution engine,
// which provides:
//   - Parallel execution with configurable worker pools
//   - Message passing between nodes across supersteps
//   - Support for distributed execution via pluggable message buses
//   - Global aggregators for cross-node coordination
//   - Efficient combiner functions to reduce message volume
//
// You can override the execution strategy by providing a custom Executor via
// WithExecutor() when compiling:
//
//	// Default: Pregel BSP execution (parallel, high performance)
//	compiled, _ := builder.Compile()
//	results := compiled.Run(ctx, initialMessages)
//
//	// Alternative: SimpleGraphExecutor (sequential, for debugging)
//	compiled, _ := builder.Compile(WithExecutor(NewSimpleGraphExecutor()))
//	results := compiled.Run(ctx, initialMessages)
//
// CONCURRENCY:
//
// Run() serializes execution via invokeMu (one invocation at a time per Compiled instance).
// This ensures state consistency but prevents concurrent runs on the same Compiled.
// For concurrent execution, either:
//   - Clone the state before each run
//   - Create separate Compiled instances
//   - Use distributed execution with external state management
//
// ITERATOR PROTOCOL:
//
// Returns an iterator (iter.Seq2) that yields (ExecutionResult, error) pairs.
// The iterator is lazy and executes the graph as you consume events:
//
//	for event, err := range compiled.Run(ctx, messages) {
//	    if err != nil {
//	        // Handle error
//	    }
//	    // Process event
//	}
//
// Or collect all results:
//
//	results, err := Collect(compiled.Run(ctx, messages))
func (cg *Compiled) Run(ctx context.Context, messages []message.Message, optFns ...RunOption) iter.Seq2[state.ExecutionResult, error] {
	cg.invokeMu.Lock()
	defer cg.invokeMu.Unlock()

	// Build runOptions from RunOption functions
	options := defaultRunOptions()
	for _, optFn := range optFns {
		optFn(&options)
	}

	// Convert to executor RunOptions (public API)
	// Pass internal runOptions as opaque data through RunOptions
	runOpts := &RunOptions{
		MaxIterations:  options.maxIterations,
		MaxConcurrency: options.maxConcurrency,
		RunID:          options.runID,
		internal:       &options, // Pass full options to executor
	}

	return cg.executor.Run(ctx, cg.topology(), cg.stateManager, messages, runOpts)
}

// ApplyState synchronously merges values and messages into the committed graph state.
// Intended for external systems (e.g., human-in-the-loop workflows) to inject
// updates between supersteps without bypassing the staged execution pipeline.
func (cg *Compiled) ApplyState(values map[string]any, messages []state.ExecutionResult) {
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
		RunFunc: func(ctx context.Context, s state.Writer) (*NodeResult, error) {
			// Sync parent state into subgraph state before execution
			parentValues := s.GetAll()
			if parentValues != nil {
				cg.ApplyState(parentValues, nil)
			}

			// Get parent messages to pass to subgraph
			parentMessages := state.ExtractMessages(s.MessagesSnapshot())

			// Execute the subgraph with parent messages and get final event
			_, err := Last(cg.Run(ctx, parentMessages))
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Return the subgraph's final state as updates
			// Get all accumulated events from state and convert to messages
			updates := cg.State().GetAll()
			events := cg.State().MessagesSnapshot()
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
	mapInput func(state.Reader) (map[string]any, []state.ExecutionResult),
	mapOutput func(state.Reader) (map[string]any, []state.ExecutionResult),
) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s state.Writer) (*NodeResult, error) {
			// Map parent state to subgraph input
			var inputValues map[string]any
			var inputMessages []state.ExecutionResult
			var messagesToInvoke []message.Message

			if mapInput != nil {
				inputValues, inputMessages = mapInput(s)
				if inputValues != nil || len(inputMessages) > 0 {
					cg.ApplyState(inputValues, inputMessages)
				}
				// Extract messages for Invoke
				messagesToInvoke = state.ExtractMessages(inputMessages)
			} else {
				// Get parent messages
				messagesToInvoke = state.ExtractMessages(s.MessagesSnapshot())
			} // Execute subgraph with messages and get final event
			_, err := Last(cg.Run(ctx, messagesToInvoke))
			if err != nil {
				return nil, fmt.Errorf("subgraph %q: %w", name, err)
			}

			// Map subgraph output to parent updates
			var updates map[string]any
			var messages []message.Message
			if mapOutput != nil {
				var events []state.ExecutionResult
				updates, events = mapOutput(cg.State())
				// Convert events to []message.Message
				messages = make([]message.Message, len(events))
				for i := range events {
					messages[i] = events[i].Message
				}
			} else {
				updates = cg.State().GetAll()
				// Get all accumulated events from state and convert to messages
				events := cg.State().MessagesSnapshot()
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

// checkpointResume holds execution state restored from a checkpoint.
type checkpointResume struct {
	superstep      int64
	completedNodes []string
	pausedNodes    []string
}

// restoreFromCheckpoint loads and restores checkpoint if configured.
// Returns execution state to resume from, or nil if no checkpoint was loaded.
func restoreFromCheckpoint(ctx context.Context, stateManager StateManager, options *runOptions, instrumentation *Instrumentation) (*checkpointResume, error) {
	if options.checkpointer == nil || options.runID == "" || !options.autoRestore {
		return nil, nil
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
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if chkpt == nil {
		return nil, nil
	}

	logger.Info("restoring from checkpoint",
		"run_id", options.runID,
		"superstep", chkpt.Superstep,
		"version", chkpt.Version,
		"completed_nodes", len(chkpt.CompletedNodes),
		"paused_nodes", len(chkpt.PausedNodes))

	// Trace checkpoint restore operation (if instrumentation configured)
	if instrumentation != nil {
		restoreCtx, restoreSpan := instrumentation.TraceCheckpoint(ctx, "restore", options.runID, chkpt.Superstep)
		err = restoreCheckpointState(stateManager, chkpt)
		restoreSpan.End(err)
		_ = restoreCtx // Context not used further
	} else {
		err = restoreCheckpointState(stateManager, chkpt)
	}

	if err != nil {
		logger.Error("failed to restore checkpoint",
			"run_id", options.runID,
			"superstep", chkpt.Superstep,
			"error", err)
		return nil, fmt.Errorf("failed to restore checkpoint: %w", err)
	}

	logger.Info("checkpoint restored successfully",
		"run_id", options.runID,
		"superstep", chkpt.Superstep,
		"version", chkpt.Version)

	return &checkpointResume{
		superstep:      chkpt.Superstep,
		completedNodes: chkpt.CompletedNodes,
		pausedNodes:    chkpt.PausedNodes,
	}, nil
}

// calculateMessageSize calculates the approximate size in bytes of a message
// by summing the sizes of all its parts.
func calculateMessageSize(msg message.Message) int {
	size := 0
	for _, part := range msg.Parts() {
		size += calculatePartSize(part)
	}
	return size
}

// calculatePartSize calculates the approximate size of a single message part.
func calculatePartSize(part message.Part) int {
	switch p := part.(type) {
	case message.TextPart:
		return len(p.Text)
	case message.DataPart:
		return calculateDataPartSize(p)
	case message.FilePart:
		return calculateFilePartSize(p)
	case message.FunctionCallPart:
		return calculateFunctionCallPartSize(p)
	case message.FunctionResponsePart:
		return calculateFunctionResponsePartSize(p)
	default:
		return 0
	}
}

// calculateDataPartSize approximates the size of a DataPart.
func calculateDataPartSize(p message.DataPart) int {
	size := 0
	for k, v := range p.Data {
		size += len(k)
		if v != nil {
			size += len(fmt.Sprint(v))
		}
	}
	return size
}

// calculateFilePartSize calculates the size of a FilePart.
func calculateFilePartSize(p message.FilePart) int {
	size := len(p.Name) + len(p.MimeType)
	switch fc := p.File.(type) {
	case message.FileRawBytes:
		size += len(fc.Bytes)
	case message.FileBase64:
		size += len(fc.Base64)
	case message.FilePath:
		size += len(fc.Path)
	case message.FileURI:
		size += len(fc.URI)
	}
	return size
}

// calculateFunctionCallPartSize calculates the size of a FunctionCallPart.
func calculateFunctionCallPartSize(p message.FunctionCallPart) int {
	if p.FunctionCall == nil {
		return 0
	}
	return len(p.FunctionCall.ID) + len(p.FunctionCall.Name) + len(p.FunctionCall.Arguments)
}

// calculateFunctionResponsePartSize calculates the size of a FunctionResponsePart.
func calculateFunctionResponsePartSize(p message.FunctionResponsePart) int {
	if p.FunctionResponse == nil {
		return 0
	}
	size := len(p.FunctionResponse.ID) + len(p.FunctionResponse.Name)
	if p.FunctionResponse.Response != nil {
		size += len(fmt.Sprint(p.FunctionResponse.Response))
	}
	return size
}

// validateMessages validates input messages against configured size and count limits.
// Returns a MessageValidationError if any limits are exceeded.
func validateMessages(messages []message.Message, options *runOptions) error {
	// Validate message count
	if options.maxInputMessages > 0 && len(messages) > options.maxInputMessages {
		return &MessageValidationError{
			Type:          "message_count",
			Limit:         options.maxInputMessages,
			Actual:        len(messages),
			MessageIndex:  -1,
			UnderlyingErr: ErrTooManyMessages,
		}
	}

	// Validate individual message sizes and calculate total
	totalSize := 0
	for i, msg := range messages {
		msgSize := calculateMessageSize(msg)

		// Check individual message size
		if options.maxMessageSize > 0 && msgSize > options.maxMessageSize {
			return &MessageValidationError{
				Type:          "message_size",
				Limit:         options.maxMessageSize,
				Actual:        msgSize,
				MessageIndex:  i,
				UnderlyingErr: ErrMessageTooLarge,
			}
		}

		totalSize += msgSize
	}

	// Validate total size
	if options.maxTotalSize > 0 && totalSize > options.maxTotalSize {
		return &MessageValidationError{
			Type:          "total_size",
			Limit:         options.maxTotalSize,
			Actual:        totalSize,
			MessageIndex:  -1,
			UnderlyingErr: ErrTotalSizeTooLarge,
		}
	}

	return nil
}

// setupRun prepares the execution context, instrumentation, and state for a graph run.
// It handles context validation, message preparation, checkpoint restoration, and provider setup.
// This is a package-level function to avoid circular dependencies between Compiled and executors.
func setupRun(ctx context.Context, stateManager StateManager, messages []message.Message, options *runOptions) (context.Context, *Instrumentation, *checkpointResume, error) {
	if ctx == nil {
		return nil, nil, nil, fmt.Errorf("%w", ErrNilContext)
	}
	if options.maxConcurrency < 1 {
		options.maxConcurrency = 1
	}

	// Validate input messages against configured limits
	if err := validateMessages(messages, options); err != nil {
		return nil, nil, nil, err
	}

	// Wrap input messages as ExecutionResults for internal processing
	if len(messages) > 0 {
		events := make([]state.ExecutionResult, len(messages))
		for i, msg := range messages {
			events[i] = *state.NewExecutionResult(msg, options.runID, "__input__")
		}
		if stateManager != nil {
			stateManager.ApplyUpdates(nil, events)
		}
	}

	// Attach observability providers to context
	ctx = attachProvidersToContext(ctx, *options)

	// Create instrumentation from providers
	instrumentation := createInstrumentation(*options)

	// Attempt to restore from checkpoint
	resume, err := restoreFromCheckpoint(ctx, stateManager, options, instrumentation)
	if err != nil {
		return nil, nil, nil, err
	}
	if resume != nil {
		options.initialSuperstep = resume.superstep
	}

	return ctx, instrumentation, resume, nil
}

// =============================================================================
// ConditionalEvaluator - Manages conditional edge routing
// =============================================================================

// ConditionalEvaluator manages conditional edge routing based on runtime state.
// It evaluates condition functions and activates target vertices.
type ConditionalEvaluator struct {
	mu            sync.RWMutex
	cg            Structure
	gatedVertices map[string]bool // vertices behind conditional edges
	openGates     map[string]bool // gates that have been opened
}

// NewConditionalEvaluator creates an evaluator for the given graph.
func NewConditionalEvaluator(cg Structure) *ConditionalEvaluator {
	gated := make(map[string]bool)
	for name := range cg.ConditionalGate() {
		if cg.ConditionalGate()[name] {
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
	conditionals := ce.cg.ConditionalByFrom()[source]
	if len(conditionals) == 0 {
		return nil, nil
	}

	activated := make(map[string]struct{})
	for _, conditional := range conditionals {
		if conditional.Condition == nil {
			continue
		}

		selected := conditional.Condition(ctx, ce.cg.StateManager())
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
			if validTarget && ce.cg.HasExecutable(target) {
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

// =============================================================================
// GraphTopology Interface Implementation
// =============================================================================

// Nodes returns the node registry map.
func (cg *Compiled) Nodes() map[string]*Node {
	return cg.nodes
}

// Outgoing returns the outgoing edges map.
func (cg *Compiled) Outgoing() map[string][]string {
	return cg.outgoing
}

// Incoming returns the incoming edges count map.
func (cg *Compiled) Incoming() map[string]int {
	return cg.incoming
}

// ConditionalByFrom returns conditional edges grouped by source node.
func (cg *Compiled) ConditionalByFrom() map[string][]ConditionalEdges {
	return cg.conditionalByFrom
}

// ConditionalGate returns the conditional gate map.
func (cg *Compiled) ConditionalGate() map[string]bool {
	return cg.conditionalGate
}

// NodeNames returns the sorted list of node names.
func (cg *Compiled) NodeNames() []string {
	return cg.nodeNames
}

// StartKey returns the start node name.
func (cg *Compiled) StartKey() string {
	return cg.startKey
}

// EndKey returns the end node name.
func (cg *Compiled) EndKey() string {
	return cg.endKey
}

// StateManager returns the graph's state manager.
func (cg *Compiled) StateManager() StateManager {
	return cg.stateManager
}

// HasExecutable checks if a node with the given name exists.
func (cg *Compiled) HasExecutable(name string) bool {
	return cg.hasExecutable(name)
}

// MarkCompleted marks a node as completed in the execution state.
func (cg *Compiled) MarkCompleted(name string) {
	cg.markCompleted(name)
}

// MarkPaused marks a node as paused in the execution state.
func (cg *Compiled) MarkPaused(name string) {
	cg.markPaused(name)
}

// ClearPaused clears the paused state for a node.
func (cg *Compiled) ClearPaused(name string) {
	cg.clearPaused(name)
}

// SetCurrentSuperstep sets the current execution superstep.
func (cg *Compiled) SetCurrentSuperstep(step int64) {
	cg.setCurrentSuperstep(step)
}

// CreateCheckpoint creates a checkpoint snapshot.
func (cg *Compiled) CreateCheckpoint(runID string, superstep int64, metadata map[string]any) *checkpoint.Checkpoint {
	return cg.createCheckpoint(runID, superstep, metadata)
}

// BootstrapScheduler initializes the scheduler with persisted state.
func (cg *Compiled) BootstrapScheduler(ctx context.Context, s *vertexScheduler) {
	cg.bootstrapScheduler(ctx, s)
}
