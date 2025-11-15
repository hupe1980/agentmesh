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
// from the concrete compiled implementation, enabling better testability and extensibility.
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

// compiledImpl is an immutable, validated graph ready for execution.
// It contains the topology (nodes, edges, conditionals) and runtime execution state.
// compiledImpl is safe for concurrent use across multiple goroutines.
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
// Architecture Note:
//
// This is the internal implementation that handles all graph execution logic.
// User-facing code interacts with Compiled[I, O], which is a generic wrapper
// providing type-safe input/output conversion.
//
// Why two layers?
//  1. Go limitation: Methods can't have type parameters, so we need standalone Compile[I,O]()
//  2. Agent APIs: Agents return compiledImpl as MessageRunnable without exposing generics
//  3. Internal sharing: Components work with concrete types, no forced generics
//  4. Separation of concerns: Type conversion (wrapper) vs execution (this implementation)
//
// See GENERICS.md for detailed rationale.
type compiledImpl struct {
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

// Compile-time check that compiledImpl implements MessageRunnable.
var _ MessageRunnable = (*compiledImpl)(nil)

func (cg *compiledImpl) hasExecutable(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := cg.nodes[name]; ok {
		return true
	}
	return false
}

func (cg *compiledImpl) markCompleted(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.markCompleted(name)
	}
}

func (cg *compiledImpl) markPaused(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.markPaused(name)
	}
}

func (cg *compiledImpl) clearPaused(name string) {
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()
	if runtime != nil {
		runtime.clearPaused(name)
	}
}

func (cg *compiledImpl) setCurrentSuperstep(step int64) {
	cg.runtimeMu.Lock()
	defer cg.runtimeMu.Unlock()
	cg.runtime = ensureExecutionState(cg.runtime)
	cg.runtime.setSuperstep(step)
}

// CurrentSuperstep returns the current execution superstep.
func (cg *compiledImpl) CurrentSuperstep() int64 {
	cg.runtimeMu.RLock()
	defer cg.runtimeMu.RUnlock()
	if cg.runtime == nil {
		return 0
	}
	return cg.runtime.currentSuperstep()
}

// State returns the current graph state (for testing and diagnostics).
// In v2.0, this returns the StateManager interface instead of *State.
func (cg *compiledImpl) State() StateManager {
	return cg.stateManager
}

// topology creates an ExecutorTopology snapshot for executors.
// This provides a clean abstraction of the graph structure without exposing
// internal Compiled fields.
func (cg *compiledImpl) topology() *ExecutorTopology {
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

func (cg *compiledImpl) bootstrapScheduler(ctx context.Context, s *vertexScheduler) {
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
func (cg *compiledImpl) Run(ctx context.Context, messages []message.Message, optFns ...RunOption) iter.Seq2[state.ExecutionResult, error] {
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
func (cg *compiledImpl) ApplyState(values map[string]any, messages []state.ExecutionResult) {
	if cg == nil || cg.stateManager == nil {
		return
	}
	cg.stateManager.ApplyUpdates(values, messages)
}

// AsNode wraps this Compiled as a Node that can be embedded in another graph.
// This enables subgraph composition and modular workflow construction.
// The subgraph's state is synchronized with the parent state before execution.
func (cg *compiledImpl) AsNode(name string) *Node {
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
func (cg *compiledImpl) AsNodeWithStateMapping(
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
func (cg *compiledImpl) Nodes() map[string]*Node {
	return cg.nodes
}

// Outgoing returns the outgoing edges map.
func (cg *compiledImpl) Outgoing() map[string][]string {
	return cg.outgoing
}

// Incoming returns the incoming edges count map.
func (cg *compiledImpl) Incoming() map[string]int {
	return cg.incoming
}

// ConditionalByFrom returns conditional edges grouped by source node.
func (cg *compiledImpl) ConditionalByFrom() map[string][]ConditionalEdges {
	return cg.conditionalByFrom
}

// ConditionalGate returns the conditional gate map.
func (cg *compiledImpl) ConditionalGate() map[string]bool {
	return cg.conditionalGate
}

// NodeNames returns the sorted list of node names.
func (cg *compiledImpl) NodeNames() []string {
	return cg.nodeNames
}

// StartKey returns the start node name.
func (cg *compiledImpl) StartKey() string {
	return cg.startKey
}

// EndKey returns the end node name.
func (cg *compiledImpl) EndKey() string {
	return cg.endKey
}

// StateManager returns the graph's state manager.
func (cg *compiledImpl) StateManager() StateManager {
	return cg.stateManager
}

// HasExecutable checks if a node with the given name exists.
func (cg *compiledImpl) HasExecutable(name string) bool {
	return cg.hasExecutable(name)
}

// MarkCompleted marks a node as completed in the execution state.
func (cg *compiledImpl) MarkCompleted(name string) {
	cg.markCompleted(name)
}

// MarkPaused marks a node as paused in the execution state.
func (cg *compiledImpl) MarkPaused(name string) {
	cg.markPaused(name)
}

// ClearPaused clears the paused state for a node.
func (cg *compiledImpl) ClearPaused(name string) {
	cg.clearPaused(name)
}

// SetCurrentSuperstep sets the current execution superstep.
func (cg *compiledImpl) SetCurrentSuperstep(step int64) {
	cg.setCurrentSuperstep(step)
}

// CreateCheckpoint creates a checkpoint snapshot (delegates to createCheckpoint implementation in options.go).
func (cg *compiledImpl) CreateCheckpoint(runID string, superstep int64, metadata map[string]any) *checkpoint.Checkpoint {
	// The actual implementation is in options.go as compiledImpl.createCheckpoint
	// This public method provides Structure interface compatibility
	return cg.createCheckpoint(runID, superstep, metadata)
}

// BootstrapScheduler initializes the scheduler with persisted state.
func (cg *compiledImpl) BootstrapScheduler(ctx context.Context, s *vertexScheduler) {
	cg.bootstrapScheduler(ctx, s)
}

// Compiled is a generic compiled graph providing end-to-end type safety.
//
// Architecture:
//
//	Compiled[I, O] is a thin type-safe wrapper around compiledImpl.
//	It provides compile-time type checking for inputs/outputs while delegating
//	all execution logic to the internal implementation.
//
//	Why this design? See GENERICS.md "Architecture Analysis: Why Two Layers?"
//	TL;DR: Go's type system requires this pattern for clean APIs with generic methods.
//
// Type Parameters:
//   - I: Input type passed to Run(). Common types:
//   - []message.Message (for message-based agents)
//   - map[string]any (for state-based graphs)
//   - string (for simple text processing)
//   - O: Output type returned by Run(). Common types:
//   - state.ExecutionResult (for agent responses)
//   - message.Message (for single message output)
//   - string (for text output)
//
// Compiled implements the Runnable[I, O] interface with full compile-time type checking.
//
// Example usage:
//
//	// Common case: Use the helper method
//	builder := graph.NewBuilder()
//	// ... configure graph
//	compiled, err := builder.CompileMessageRunnable()
//
//	// Custom types: Use standalone generic function
//	compiled, err := graph.Compile[MyInput, MyOutput](builder)
//
//	// Type-safe execution - no type assertions needed
//	messages := []message.Message{message.NewUserMessage("Hello")}
//	for result, err := range compiled.Run(ctx, messages) {
//	    // result is state.ExecutionResult (not any)
//	}
//
// Thread Safety:
//
//	Compiled is safe for concurrent use. Multiple goroutines can call
//	Run() simultaneously - each execution gets independent state.
type Compiled[I, O any] struct {
	inner *compiledImpl // Internal implementation

	// Type conversion functions
	inputConverter  func(I) ([]message.Message, error)
	outputConverter func(state.ExecutionResult) (O, error)
}

// Run executes the graph with the given input and returns a type-safe iterator.
// Unlike Compiled.Run(), this method requires no type assertions on input or output.
//
// The input type I is enforced at compile time, eliminating runtime type errors.
// The output type O is guaranteed by the type system.
//
// Example:
//
//	compiled, _ := builder.Compile[[]message.Message, state.ExecutionResult]()
//	messages := []message.Message{...}
//	for result, err := range compiled.Run(ctx, messages) {
//	    if err != nil {
//	        return err
//	    }
//	    // result is state.ExecutionResult - no casting needed!
//	    fmt.Println(result.Messages)
//	}
func (cg *Compiled[I, O]) Run(ctx context.Context, input I, optFns ...RunOption) iter.Seq2[O, error] {
	return func(yield func(O, error) bool) {
		// Convert input using type-safe converter
		messages, err := cg.inputConverter(input)
		if err != nil {
			var zero O
			yield(zero, fmt.Errorf("input conversion failed: %w", err))
			return
		}

		// Execute inner compiled graph
		for result, err := range cg.inner.Run(ctx, messages, optFns...) {
			if err != nil {
				var zero O
				if !yield(zero, err) {
					return
				}
				continue
			}

			// Convert output using type-safe converter
			output, err := cg.outputConverter(result)
			if err != nil {
				var zero O
				if !yield(zero, fmt.Errorf("output conversion failed: %w", err)) {
					return
				}
				continue
			}

			if !yield(output, nil) {
				return
			}
		}
	}
}

// State returns the current graph state manager.
// This provides access to state for testing and diagnostics.
func (cg *Compiled[I, O]) State() StateManager {
	return cg.inner.State()
}

// CurrentSuperstep returns the current execution superstep.
func (cg *Compiled[I, O]) CurrentSuperstep() int64 {
	return cg.inner.CurrentSuperstep()
}

// ApplyState synchronously merges values and messages into the committed graph state.
func (cg *Compiled[I, O]) ApplyState(values map[string]any, messages []state.ExecutionResult) {
	cg.inner.ApplyState(values, messages)
}

// AsNode wraps this CompiledGeneric as a Node for subgraph composition.
func (cg *Compiled[I, O]) AsNode(name string) *Node {
	return cg.inner.AsNode(name)
}

// AsNodeWithStateMapping wraps this CompiledGeneric as a Node with custom state mapping.
func (cg *Compiled[I, O]) AsNodeWithStateMapping(
	name string,
	mapInput func(state.Reader) (map[string]any, []state.ExecutionResult),
	mapOutput func(state.Reader) (map[string]any, []state.ExecutionResult),
) *Node {
	return cg.inner.AsNodeWithStateMapping(name, mapInput, mapOutput)
}

// Nodes returns the node registry (implements Structure interface).
func (cg *Compiled[I, O]) Nodes() map[string]*Node {
	return cg.inner.Nodes()
}

// Outgoing returns outgoing edges (implements Structure interface).
func (cg *Compiled[I, O]) Outgoing() map[string][]string {
	return cg.inner.Outgoing()
}

// Incoming returns incoming edge counts (implements Structure interface).
func (cg *Compiled[I, O]) Incoming() map[string]int {
	return cg.inner.Incoming()
}

// ConditionalByFrom returns conditional edges grouped by source (implements Structure interface).
func (cg *Compiled[I, O]) ConditionalByFrom() map[string][]ConditionalEdges {
	return cg.inner.ConditionalByFrom()
}

// ConditionalGate returns conditional gate status (implements Structure interface).
func (cg *Compiled[I, O]) ConditionalGate() map[string]bool {
	return cg.inner.ConditionalGate()
}

// NodeNames returns sorted node names (implements Structure interface).
func (cg *Compiled[I, O]) NodeNames() []string {
	return cg.inner.NodeNames()
}

// StartKey returns the start node name (implements Structure interface).
func (cg *Compiled[I, O]) StartKey() string {
	return cg.inner.StartKey()
}

// EndKey returns the end node name (implements Structure interface).
func (cg *Compiled[I, O]) EndKey() string {
	return cg.inner.EndKey()
}

// StateManager returns the state manager (implements Structure interface).
func (cg *Compiled[I, O]) StateManager() StateManager {
	return cg.inner.StateManager()
}

// HasExecutable checks if a node exists (implements Structure interface).
func (cg *Compiled[I, O]) HasExecutable(name string) bool {
	return cg.inner.HasExecutable(name)
}

// MarkCompleted marks a node as completed (implements Structure interface).
func (cg *Compiled[I, O]) MarkCompleted(name string) {
	cg.inner.MarkCompleted(name)
}

// MarkPaused marks a node as paused (implements Structure interface).
func (cg *Compiled[I, O]) MarkPaused(name string) {
	cg.inner.MarkPaused(name)
}

// ClearPaused clears the paused state (implements Structure interface).
func (cg *Compiled[I, O]) ClearPaused(name string) {
	cg.inner.ClearPaused(name)
}

// SetCurrentSuperstep sets the superstep (implements Structure interface).
func (cg *Compiled[I, O]) SetCurrentSuperstep(step int64) {
	cg.inner.SetCurrentSuperstep(step)
}

// CreateCheckpoint creates a checkpoint snapshot (implements Structure interface).
func (cg *Compiled[I, O]) CreateCheckpoint(runID string, superstep int64, metadata map[string]any) *checkpoint.Checkpoint {
	return cg.inner.CreateCheckpoint(runID, superstep, metadata)
}

// BootstrapScheduler initializes the scheduler (implements Structure interface).
func (cg *Compiled[I, O]) BootstrapScheduler(ctx context.Context, s *vertexScheduler) {
	cg.inner.BootstrapScheduler(ctx, s)
}

// Compile-time checks that Compiled[I, O] implements Runnable[I, O] for common types.
// These ensure the interface is correctly implemented during compilation.
var (
	_ MessageRunnable = (*Compiled[[]message.Message, state.ExecutionResult])(nil)
	_ StateRunnable   = (*Compiled[map[string]any, state.ExecutionResult])(nil)
	_ StringRunnable  = (*Compiled[string, string])(nil)

	// Verify StatefulRunnable implementation
	_ StatefulRunnable[[]message.Message, state.ExecutionResult] = (*Compiled[[]message.Message, state.ExecutionResult])(nil)
)

// NewCompiled creates a Compiled[I, O] from an internal compiled implementation.
// This is used internally by compilation functions to wrap the implementation with type safety.
func NewCompiled[I, O any](inner *compiledImpl) *Compiled[I, O] {
	return &Compiled[I, O]{
		inner:           inner,
		inputConverter:  createInputConverter[I](),
		outputConverter: createOutputConverter[O](),
	}
}

// =============================================================================
// Introspection Methods - Delegate to inner implementation
// =============================================================================

// GetNodes returns the list of all node names in the graph.
func (cg *Compiled[I, O]) GetNodes() []string {
	return cg.inner.GetNodes()
}

// GetNodeInfo returns detailed information about a specific node.
func (cg *Compiled[I, O]) GetNodeInfo(name string) (*NodeInfo, error) {
	return cg.inner.GetNodeInfo(name)
}

// GetAllNodeInfo returns information about all nodes in the graph.
func (cg *Compiled[I, O]) GetAllNodeInfo() []NodeInfo {
	return cg.inner.GetAllNodeInfo()
}

// GetEdges returns information about all edges in the graph.
func (cg *Compiled[I, O]) GetEdges() []EdgeInfo {
	return cg.inner.GetEdges()
}

// GetTopology returns the complete graph topology.
func (cg *Compiled[I, O]) GetTopology() *Topology {
	return cg.inner.GetTopology()
}

// GetMetrics returns graph metrics.
func (cg *Compiled[I, O]) GetMetrics() *Metrics {
	return cg.inner.GetMetrics()
}

// GetDependencies returns the dependencies for a specific node.
func (cg *Compiled[I, O]) GetDependencies(name string) (*NodeDependencies, error) {
	return cg.inner.GetDependencies(name)
}

// GetExecutionPath returns possible execution paths through the graph.
func (cg *Compiled[I, O]) GetExecutionPath(maxPaths int) [][]string {
	return cg.inner.GetExecutionPath(maxPaths)
}

// GenerateMermaidFlowchart generates a Mermaid flowchart diagram of the graph.
func (cg *Compiled[I, O]) GenerateMermaidFlowchart(direction string) string {
	return cg.inner.GenerateMermaidFlowchart(direction)
}

// Test helpers - delegate to inner for test-only methods
func (cg *Compiled[I, O]) calculateDepth(name string) int {
	return cg.inner.calculateDepth(name)
}

func (cg *Compiled[I, O]) findAllPredecessors(name string) []string {
	return cg.inner.findAllPredecessors(name)
}

func (cg *Compiled[I, O]) findAllSuccessors(name string) []string {
	return cg.inner.findAllSuccessors(name)
}
