package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// wrapTimeoutError wraps context.DeadlineExceeded errors as NodeTimeoutError for consistency.
func wrapTimeoutError(ctx context.Context, err error) error {
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// Check if already wrapped (node-level timeout)
	var nodeTimeoutErr *NodeTimeoutError
	if errors.As(err, &nodeTimeoutErr) {
		return err
	}

	// Not wrapped yet - this is a runtime-level timeout
	timeout := int64(0)
	if deadline, ok := ctx.Deadline(); ok {
		elapsed := time.Since(deadline)
		if elapsed > 0 {
			timeout = int64(elapsed / time.Millisecond)
		}
	}

	return &NodeTimeoutError{
		Node:    "", // Runtime-level timeout (not node-specific)
		Timeout: timeout,
		Cause:   err,
	}
}

// =============================================================================
// ChannelMessage - Data payload for Pregel BSP
// =============================================================================

// ChannelMessage is the data-carrying message payload for Pregel BSP execution.
// It contains actual data to be communicated between nodes via channels.
type ChannelMessage struct {
	// Messages contains message events with execution metadata
	Messages []stateif.ExecutionResult `json:"messages,omitzero"`

	// Updates contains key-value state updates to be applied to channels
	Updates map[string]any `json:"updates,omitzero"`

	// Metadata contains additional routing or processing hints
	Metadata map[string]string `json:"metadata,omitzero"`
}

// NewChannelMessage creates a new channel message with the given message events and updates.
func NewChannelMessage(messages []stateif.ExecutionResult, updates map[string]any) ChannelMessage {
	return ChannelMessage{
		Messages: messages,
		Updates:  updates,
		Metadata: make(map[string]string),
	}
}

// WithMetadata adds metadata to the channel message.
func (cm ChannelMessage) WithMetadata(key, value string) ChannelMessage {
	if cm.Metadata == nil {
		cm.Metadata = make(map[string]string)
	}
	cm.Metadata[key] = value
	return cm
}

// MarshalJSON implements json.Marshaler for serialization.
func (cm ChannelMessage) MarshalJSON() ([]byte, error) {
	type Alias ChannelMessage
	return json.Marshal(struct {
		Type string `json:"type"`
		Alias
	}{
		Type:  "channel_message",
		Alias: (Alias)(cm),
	})
}

// UnmarshalJSON implements json.Unmarshaler for deserialization.
func (cm *ChannelMessage) UnmarshalJSON(data []byte) error {
	type Alias ChannelMessage
	aux := &struct {
		Type string `json:"type"`
		*Alias
	}{
		Alias: (*Alias)(cm),
	}
	return json.Unmarshal(data, &aux)
}

// IsEmpty returns true if the message contains no data.
func (cm ChannelMessage) IsEmpty() bool {
	return len(cm.Messages) == 0 && len(cm.Updates) == 0 && len(cm.Metadata) == 0
}

// Clone creates a deep copy of the channel message.
func (cm ChannelMessage) Clone() ChannelMessage {
	clone := ChannelMessage{
		Metadata: make(map[string]string, len(cm.Metadata)),
	}

	if len(cm.Messages) > 0 {
		clone.Messages = make([]stateif.ExecutionResult, len(cm.Messages))
		copy(clone.Messages, cm.Messages)
	}

	if len(cm.Updates) > 0 {
		clone.Updates = make(map[string]any, len(cm.Updates))
		for k, v := range cm.Updates {
			clone.Updates[k] = v
		}
	}

	for k, v := range cm.Metadata {
		clone.Metadata[k] = v
	}

	return clone
}

// =============================================================================
// Pregel Runtime - BSP execution coordinator
// =============================================================================

// graphRuntime coordinates graph execution by mediating between the scheduler
// (topology + routing logic) and the Pregel BSP engine. It implements the
// Coordinator pattern, owning both components and ensuring clean separation
// of concerns. The scheduler determines WHAT to run, the engine determines HOW
// to run it, and graphRuntime orchestrates the interaction.
//
// Architecture:
//   - Scheduler: Manages graph topology, conditional edges, and execution order
//   - Pregel Engine: Executes nodes in parallel BSP supersteps
//   - graphRuntime: Coordinates interaction and manages lifecycle
//
// This separation allows the pure BSP engine (pkg/pregel) to remain
// domain-agnostic while graph-specific concerns (channels, checkpoints,
// conditional routing) are handled at the graph layer.
//
// The pkg/pregel package is now public API, enabling advanced users to
// implement custom MessageBus backends, schedulers, and fine-tune the
// execution engine for specific use cases.
//
// =============================================================================
// Sub-Coordinators - Extracted responsibilities from graphRuntime
// =============================================================================

// stateCoordinator manages state persistence and asynchronous checkpointing.
// It handles checkpoint queue management and background checkpoint saves.
type stateCoordinator struct {
	stateManager StateManager
	checkpointer checkpoint.Checkpointer
	options      struct {
		runID                 string
		checkpointInterval    int
		failOnCheckpointError bool
	}
	checkpointQueue chan *checkpoint.Checkpoint
	checkpointWG    sync.WaitGroup
}

// newStateCoordinator creates a new state coordinator for managing checkpoints.
func newStateCoordinator(sm StateManager, checkpointer checkpoint.Checkpointer, runID string, interval int, failOnError bool) *stateCoordinator {
	sc := &stateCoordinator{
		stateManager: sm,
		checkpointer: checkpointer,
	}
	sc.options.runID = runID
	sc.options.checkpointInterval = interval
	sc.options.failOnCheckpointError = failOnError
	return sc
}

// startCheckpointWorker initializes and starts the asynchronous checkpoint worker.
func (sc *stateCoordinator) startCheckpointWorker(ctx context.Context, onError func(error)) {
	logger := logging.FromContext(ctx)

	if sc.checkpointer == nil || sc.options.runID == "" {
		return
	}
	if sc.checkpointQueue != nil {
		return
	}

	logger.Debug("starting async checkpoint worker",
		"run_id", sc.options.runID,
		"queue_size", 1)

	sc.checkpointQueue = make(chan *checkpoint.Checkpoint, 1)
	// Create detached context for background saves (checkpoints must complete even if request canceled)
	saveCtx := context.WithoutCancel(ctx)
	sc.checkpointWG.Add(1)

	go func() {
		defer sc.checkpointWG.Done()
		logger := logging.FromContext(saveCtx)

		for checkpoint := range sc.checkpointQueue {
			if checkpoint == nil {
				continue
			}

			logger.Debug("processing checkpoint from queue",
				"run_id", checkpoint.RunID,
				"superstep", checkpoint.Superstep)

			if err := sc.checkpointer.Save(saveCtx, checkpoint); err != nil {
				checkpointErr := fmt.Errorf("failed to save checkpoint at superstep %d: %w", checkpoint.Superstep, err)
				logger.Error("async checkpoint save failed",
					"run_id", checkpoint.RunID,
					"superstep", checkpoint.Superstep,
					"error", err)
				onError(checkpointErr)
			} else {
				logger.Info("async checkpoint saved successfully",
					"run_id", checkpoint.RunID,
					"superstep", checkpoint.Superstep,
					"version", checkpoint.Version)
			}
		}

		logger.Debug("checkpoint worker stopped", "run_id", sc.options.runID)
	}()
}

// stopCheckpointWorker gracefully shuts down the checkpoint worker.
func (sc *stateCoordinator) stopCheckpointWorker() {
	if sc.checkpointQueue == nil {
		return
	}
	close(sc.checkpointQueue)
	sc.checkpointWG.Wait()
	sc.checkpointQueue = nil
}

// queueCheckpoint adds a checkpoint to the async save queue with backpressure handling.
func (sc *stateCoordinator) queueCheckpoint(ctx context.Context, checkpoint *checkpoint.Checkpoint, onError func(error)) {
	logger := logging.FromContext(ctx)

	if sc.checkpointQueue == nil {
		return
	}

	select {
	case sc.checkpointQueue <- checkpoint:
		logger.Debug("checkpoint queued for async save",
			"run_id", checkpoint.RunID,
			"superstep", checkpoint.Superstep)
	default:
		// Queue is full - checkpoint worker is busy processing previous save
		logger.Warn("checkpoint queue full, waiting for worker",
			"run_id", checkpoint.RunID,
			"superstep", checkpoint.Superstep)

		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()

		select {
		case sc.checkpointQueue <- checkpoint:
			logger.Debug("checkpoint queued after wait",
				"run_id", checkpoint.RunID,
				"superstep", checkpoint.Superstep)
		case <-timer.C:
			queueErr := fmt.Errorf("checkpoint queue timeout at superstep %d after 5s: checkpoint dropped", checkpoint.Superstep)
			logger.Error("checkpoint queue timeout",
				"run_id", checkpoint.RunID,
				"superstep", checkpoint.Superstep)
			onError(queueErr)
		case <-ctx.Done():
			logger.Warn("checkpoint save cancelled",
				"run_id", checkpoint.RunID,
				"superstep", checkpoint.Superstep)
		}
	}
}

// eventEmitter manages event emission and observability instrumentation.
// It handles yield function calls, instrumentation tracing, and event stream control.
type eventEmitter struct {
	yield           func(stateif.ExecutionResult, error) bool
	instrumentation *Instrumentation
	cancel          context.CancelFunc
	yieldMu         sync.Mutex
	yieldStopped    bool
	errOnce         sync.Once
}

// newEventEmitter creates a new event emitter for managing execution events.
func newEventEmitter(yield func(stateif.ExecutionResult, error) bool, instrumentation *Instrumentation, cancel context.CancelFunc) *eventEmitter {
	return &eventEmitter{
		yield:           yield,
		instrumentation: instrumentation,
		cancel:          cancel,
	}
}

// emit sends an execution result through the yield function with proper synchronization.
func (ee *eventEmitter) emit(event stateif.ExecutionResult) {
	if ee.yield == nil {
		return
	}

	ee.yieldMu.Lock()
	defer ee.yieldMu.Unlock()

	// Don't call yield if it already returned false
	if ee.yieldStopped {
		return
	}

	// Call yield and mark as stopped if it returns false
	if !ee.yield(event, event.Err) {
		ee.yieldStopped = true
		if ee.cancel != nil {
			ee.cancel()
		}
	}
}

// fail emits a terminal error event and cancels execution (once only).
func (ee *eventEmitter) fail(err error) {
	if err == nil {
		return
	}
	ee.errOnce.Do(func() {
		ee.emit(stateif.ExecutionResult{Err: err})
		if ee.cancel != nil {
			ee.cancel()
		}
	})
}

// emitError emits a non-terminal error event without cancelling execution.
func (ee *eventEmitter) emitError(err error) {
	if err == nil {
		return
	}
	ee.emit(stateif.ExecutionResult{Err: err})
}

// traceGraphExecution starts a trace span for graph execution.
func (ee *eventEmitter) traceGraphExecution(ctx context.Context, name string) (context.Context, trace.Span) {
	if ee.instrumentation == nil {
		return ctx, nil
	}
	return ee.instrumentation.TraceGraphExecution(ctx, name)
}

// traceCheckpoint starts a trace span for checkpoint operations.
func (ee *eventEmitter) traceCheckpoint(ctx context.Context, operation, runID string, superstep int64) (context.Context, trace.Span) {
	if ee.instrumentation == nil {
		return ctx, nil
	}
	return ee.instrumentation.TraceCheckpoint(ctx, operation, runID, superstep)
}

// =============================================================================
// graphRuntime - BSP Execution Coordinator
// =============================================================================

// graphRuntime coordinates graph execution with delegated responsibilities.
//
// Thread Safety:
//   - stateCoordinator: Handles checkpoint queue (channel-based, thread-safe) and WaitGroup
//   - eventEmitter: Protects yield calls with mutex, safe for concurrent emit() calls
//   - scheduler: Has internal locking, safe for concurrent access
//
// The runtime now delegates to sub-coordinators instead of managing all concerns directly:
//   - stateCoordinator: Checkpoint persistence and async saves
//   - eventEmitter: Event emission, yield management, and observability tracing
type graphRuntime struct {
	cg      *Compiled
	options runOptions

	scheduler        *vertexScheduler                              // Graph topology & routing
	engine           *pregel.Runtime[StateManager, ChannelMessage] // BSP execution engine
	stateCoordinator *stateCoordinator                             // State persistence & checkpointing
	eventEmitter     *eventEmitter                                 // Event emission & observability
}

// compiledPregelGraph adapts Compiled to the pregel.Graph interface.
// This allows the Pregel runtime to execute graph nodes without knowing about
// agent-specific concepts like channels, checkpoints, or conditional routing.
//
// The adapter pattern is used here to bridge between:
//   - Graph domain (StateManager, ChannelMessage, Node)
//   - Pregel domain (Graph[S, M], Node[S, M], Message[M])
type compiledPregelGraph struct {
	runtime *graphRuntime
}

// nodeAdapter wraps a graph.Node as a pregel.Node.
// It handles the translation between graph-level execution (with retry policies,
// rate limiting, timeout wrapping, and state buffering) and Pregel-level
// vertex execution (pure computation with message passing).
//
// Responsibilities:
//   - Execute node with retry policy if configured
//   - Apply rate limiting if configured for this node
//   - Wrap timeout errors in NodeTimeoutError for better diagnostics
//   - Buffer aggregation calls during execution
//   - Route outgoing messages via scheduler
//   - Update graph state and emit events
type nodeAdapter struct {
	runtime *graphRuntime
	name    string
	node    *Node
}

// wrapMessagesAsEvents wraps raw messages from node execution in stateif.ExecutionResult structs
// with execution metadata (graphID, nodeName, timestamp, UUID).
func (n *nodeAdapter) wrapMessagesAsEvents(messages []message.Message) []stateif.ExecutionResult {
	if len(messages) == 0 {
		return nil
	}

	graphID := ""
	if n.runtime != nil {
		graphID = n.runtime.options.runID
	}

	events := make([]stateif.ExecutionResult, len(messages))
	for i, msg := range messages {
		events[i] = *stateif.NewExecutionResult(msg, graphID, n.name)
	}

	return events
}

func newPregelRuntime(cg *Compiled, cancel context.CancelFunc, options runOptions, yield func(stateif.ExecutionResult, error) bool, instrumentation *Instrumentation) *graphRuntime {
	scheduler := newVertexScheduler(cg)

	// Create sub-coordinators
	var stateCoord *stateCoordinator
	if cg != nil {
		stateCoord = newStateCoordinator(
			cg.stateManager,
			options.checkpointer,
			options.runID,
			options.checkpointInterval,
			options.failOnCheckpointError,
		)
	}
	eventEmit := newEventEmitter(yield, instrumentation, cancel)

	gr := &graphRuntime{
		cg:               cg,
		options:          options,
		scheduler:        scheduler,
		stateCoordinator: stateCoord,
		eventEmitter:     eventEmit,
	}

	// Note: maxMessages is now configured at StateManager creation time via NewStateManager(maxMessages).
	// The message limit cannot be changed after the state is created.

	adapter := &compiledPregelGraph{runtime: gr}
	maxWorkers := max(options.maxConcurrency, 1)
	if cg != nil {
		cg.setCurrentSuperstep(options.initialSuperstep)
	}
	runtimeOptions := []pregel.RuntimeOption[StateManager, ChannelMessage]{
		pregel.WithMaxWorkers[StateManager, ChannelMessage](maxWorkers),
		pregel.WithInitialSuperstep[StateManager, ChannelMessage](options.initialSuperstep),
	}
	if options.maxIterations > 0 {
		runtimeOptions = append(runtimeOptions, pregel.WithMaxIterations[StateManager, ChannelMessage](options.maxIterations))
	}
	if len(options.aggregators) > 0 {
		runtimeOptions = append(runtimeOptions, pregel.WithAggregators[StateManager, ChannelMessage](options.aggregators))
	}
	if options.combiner != nil {
		runtimeOptions = append(runtimeOptions, pregel.WithCombiner[StateManager, ChannelMessage](adaptCombiner(options.combiner)))
	}
	// Use custom message bus if provided (enables distributed execution)
	if options.messageBus != nil {
		runtimeOptions = append(runtimeOptions, pregel.WithMessageBus[StateManager, ChannelMessage](options.messageBus))
	}
	// Install checkpoint callback if configured
	if options.checkpointer != nil && options.runID != "" && options.checkpointInterval > 0 {
		runtimeOptions = append(runtimeOptions, pregel.WithOnSuperstepComplete[StateManager, ChannelMessage](func(ctx context.Context, superstep int64) {
			// Context is now passed directly from the pregel runtime
			gr.saveCheckpoint(ctx, superstep)
		}))
	}

	// Create the Pregel runtime (use MustNewRuntime since inputs are already validated)
	gr.engine = pregel.MustNewRuntime(adapter, nil, runtimeOptions...)

	// Note: checkpoint worker will be started in run() method with context

	return gr
}

func (gr *graphRuntime) startCheckpointWorker(ctx context.Context) {
	if gr.stateCoordinator == nil {
		return
	}
	onError := func(err error) {
		if gr.stateCoordinator.options.failOnCheckpointError {
			gr.eventEmitter.fail(err)
		} else {
			gr.eventEmitter.emitError(err)
		}
	}
	gr.stateCoordinator.startCheckpointWorker(ctx, onError)
}

func (gr *graphRuntime) stopCheckpointWorker() {
	if gr.stateCoordinator == nil {
		return
	}
	gr.stateCoordinator.stopCheckpointWorker()
}

// run executes the graph runtime, coordinating checkpointing, tracing, and execution.
func (gr *graphRuntime) run(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	startTime := time.Now()

	logger.Info("starting graph execution",
		"run_id", gr.options.runID,
		"checkpoint_interval", gr.options.checkpointInterval)

	// Setup checkpoint worker and tracing
	ctx, cleanup := gr.setupExecution(ctx)
	defer cleanup()

	// Bootstrap and run engine
	if gr.cg != nil {
		gr.cg.bootstrapScheduler(ctx, gr.scheduler)
	}

	err := gr.engine.Run(ctx)

	if gr.cg != nil {
		gr.cg.setCurrentSuperstep(gr.engine.Stats().Supersteps)
	}

	// Finalize execution and log results
	return gr.finalizeExecution(ctx, err, logger, startTime)
}

// setupExecution prepares the runtime for execution by starting checkpoint worker and tracing.
// Returns the traced context and a cleanup function that must be called when done.
func (gr *graphRuntime) setupExecution(ctx context.Context) (context.Context, func()) {
	// Start checkpoint worker with this request's context
	gr.startCheckpointWorker(ctx)

	// Start runtime trace span
	ctx, span := gr.eventEmitter.traceGraphExecution(ctx, "runtime.run")

	cleanup := func() {
		if span != nil {
			span.End(nil)
		}
		gr.stopCheckpointWorker()
	}

	return ctx, cleanup
}

// finalizeExecution handles post-execution tasks: aggregate transfer, error wrapping, and logging.
func (gr *graphRuntime) finalizeExecution(
	ctx context.Context,
	err error,
	logger logging.Logger,
	startTime time.Time,
) error {
	duration := time.Since(startTime)
	stats := gr.engine.Stats()

	// Transfer final aggregates to graph state
	if err == nil || errors.Is(err, context.Canceled) {
		if aggregates := gr.engine.Aggregates(); len(aggregates) > 0 {
			gr.cg.stateManager.SetAggregates(aggregates)
		}
	}

	// Wrap timeout errors for consistency
	err = wrapTimeoutError(ctx, err)

	// Log execution result
	gr.logExecutionResult(logger, err, stats.Supersteps, duration)

	return err
}

// logExecutionResult logs the appropriate message based on execution outcome.
func (gr *graphRuntime) logExecutionResult(
	logger logging.Logger,
	err error,
	supersteps int64,
	duration time.Duration,
) {
	logFields := []any{
		"run_id", gr.options.runID,
		"supersteps", supersteps,
		"duration_ms", duration.Milliseconds(),
	}

	switch {
	case err != nil && !errors.Is(err, context.Canceled):
		logger.Error("graph execution failed", append(logFields, "error", err)...)
		gr.fail(err)
	case errors.Is(err, context.Canceled):
		logger.Warn("graph execution canceled", logFields...)
	default:
		logger.Info("graph execution completed successfully", logFields...)
	}
}

func (gr *graphRuntime) saveCheckpoint(ctx context.Context, superstep int64) {
	logger := logging.FromContext(ctx)

	// Skip checkpoint if not configured or interval not reached
	if gr.options.checkpointer == nil || gr.options.runID == "" {
		return
	}
	if gr.options.checkpointInterval > 0 && superstep%int64(gr.options.checkpointInterval) != 0 {
		return
	}

	logger.Debug("saving checkpoint",
		"run_id", gr.options.runID,
		"superstep", superstep,
		"interval", gr.options.checkpointInterval)

	// Trace checkpoint save operation
	ctx, span := gr.eventEmitter.traceCheckpoint(ctx, "save", gr.options.runID, superstep)
	if span != nil {
		defer span.End(nil)
	}

	// Create checkpoint from current state
	checkpoint := gr.cg.createCheckpoint(gr.options.runID, superstep, nil)
	if checkpoint == nil {
		logger.Warn("failed to create checkpoint snapshot",
			"run_id", gr.options.runID,
			"superstep", superstep)
		return
	}

	// Delegate to stateCoordinator for async queueing
	if gr.stateCoordinator != nil {
		onError := func(err error) {
			if gr.options.failOnCheckpointError {
				gr.eventEmitter.fail(err)
			} else {
				gr.eventEmitter.emitError(err)
			}
		}
		gr.stateCoordinator.queueCheckpoint(ctx, checkpoint, onError)
		return
	}

	// Fallback to synchronous save if no coordinator
	if err := gr.options.checkpointer.Save(context.WithoutCancel(ctx), checkpoint); err != nil {
		checkpointErr := fmt.Errorf("failed to save checkpoint at superstep %d: %w", superstep, err)
		logger.Error("failed to save checkpoint",
			"run_id", gr.options.runID,
			"superstep", superstep,
			"error", err)
		if gr.options.failOnCheckpointError {
			gr.eventEmitter.fail(checkpointErr)
		} else {
			gr.eventEmitter.emitError(checkpointErr)
		}
	} else {
		logger.Info("checkpoint saved successfully",
			"run_id", gr.options.runID,
			"superstep", superstep,
			"version", checkpoint.Version)
	}
}

func (gr *graphRuntime) markExecuted(name string) {
	if gr.scheduler != nil {
		gr.scheduler.MarkExecuted(name)
	}
}

func (gr *graphRuntime) setPaused(name string) {
	if gr.scheduler != nil {
		gr.scheduler.MarkPaused(name)
	}
	if gr.cg != nil && gr.engine != nil {
		gr.cg.setCurrentSuperstep(gr.engine.CurrentSuperstep())
	}
}

func (gr *graphRuntime) onVertexCompleted(ctx context.Context, name string) ([]string, error) {
	return gr.scheduler.OnVertexCompleted(ctx, name)
}

func (gr *graphRuntime) emit(event stateif.ExecutionResult) {
	if gr.eventEmitter == nil {
		return
	}
	gr.eventEmitter.emit(event)
}

func (gr *graphRuntime) fail(err error) {
	if gr.eventEmitter == nil {
		return
	}
	gr.eventEmitter.fail(err)
}

func (gr *graphRuntime) emitError(err error) {
	if gr.eventEmitter == nil {
		return
	}
	gr.eventEmitter.emitError(err)
}

// compiledPregelGraph implements the pregel interfaces for Compiled.

func (g *compiledPregelGraph) RootNodes() []string {
	return g.runtime.scheduler.Ready()
}

func (g *compiledPregelGraph) Outgoing(node string) []string {
	if targets := g.runtime.cg.outgoing[node]; len(targets) > 0 {
		return append([]string(nil), targets...)
	}
	return nil
}

func (g *compiledPregelGraph) NodeByName(name string) pregel.Node[StateManager, ChannelMessage] {
	if node, ok := g.runtime.cg.nodes[name]; ok {
		return &nodeAdapter{runtime: g.runtime, name: name, node: node}
	}
	return nil
}

func (g *compiledPregelGraph) State() StateManager {
	if g.runtime.cg.stateManager == nil {
		return nil
	}
	return g.runtime.cg.stateManager
}

// nodeAdapter executes both standard and command-style nodes within the Pregel runtime.

func (n *nodeAdapter) Name() string { return n.name }

// handleScheduledDelivery handles message delivery to scheduled next nodes.
func (n *nodeAdapter) handleScheduledDelivery(ctx context.Context, messages []stateif.ExecutionResult, updates map[string]any) error {
	if n.runtime == nil || n.runtime.scheduler == nil {
		return nil
	}

	next, schedErr := n.runtime.onVertexCompleted(ctx, n.name)
	if schedErr != nil {
		return schedErr
	}

	if len(next) == 0 || n.runtime.engine == nil {
		return nil
	}

	// Create channel messages with data from node execution
	deliveries := make([]pregel.Message[ChannelMessage], 0, len(next))
	for _, target := range next {
		// Send actual data in the message (not empty signal)
		msg := NewChannelMessage(messages, updates)
		deliveries = append(deliveries, pregel.Message[ChannelMessage]{
			From: n.name,
			To:   target,
			Data: msg,
		})
	}

	// Deliver messages with backpressure - blocks if mailbox full
	if err := n.runtime.engine.Deliver(ctx, deliveries...); err != nil {
		// Delivery error (e.g., context cancelled) - this is a real error now
		return fmt.Errorf("node %q: message delivery failed: %w", n.name, err)
	}

	return nil
}

//nolint:gocyclo // Node execution requires handling many runtime conditions
func (n *nodeAdapter) Run(ctx context.Context, vertex pregel.VertexContext[StateManager, ChannelMessage], incoming []pregel.Message[ChannelMessage]) error {
	logger := logging.FromContext(ctx)

	// Start node execution trace span
	var superstep int64
	if n.runtime != nil && n.runtime.engine != nil {
		superstep = n.runtime.engine.CurrentSuperstep()
	}

	logger.Debug("starting node execution",
		"node", n.name,
		"superstep", superstep,
		"incoming_messages", len(incoming))

	startTime := time.Now()
	var span trace.Span
	if n.runtime.eventEmitter != nil && n.runtime.eventEmitter.instrumentation != nil {
		ctx, span = n.runtime.eventEmitter.instrumentation.TraceNodeExecution(ctx, n.name, superstep)
	}
	defer func() {
		// Record metrics and end span
		duration := time.Since(startTime)
		if n.runtime.eventEmitter != nil && n.runtime.eventEmitter.instrumentation != nil {
			n.runtime.eventEmitter.instrumentation.RecordNodeExecution(ctx, n.name, duration, nil)
			if span != nil {
				span.End(nil)
			}
		}
	}()

	writer := func(result *NodeResult) {
		if result == nil {
			return
		}
		// Emit one stateif.ExecutionResult per message
		if len(result.Messages) == 0 {
			// No messages: emit a single result with just Updates
			n.runtime.emit(stateif.ExecutionResult{
				Node:    n.name,
				Updates: result.Updates,
			})
		} else {
			for i, msg := range result.Messages {
				evt := stateif.NewExecutionResult(msg, n.runtime.options.runID, n.name)
				// Include Updates only in the first result
				if i == 0 {
					evt.Updates = result.Updates
				}
				n.runtime.emit(*evt)
			}
		}
	}
	nodeCtx := withStreamWriter(ctx, writer)

	// Create buffered state writer to prevent mutations from being visible
	// within the same superstep (maintains BSP semantics)
	var bufferedState stateif.Writer
	if vertex.State != nil {
		vertex.State.SetAggregates(vertex.Aggregates)
		vertex.State.SetAggregateFn(vertex.Aggregate)
		bufferedState = newBufferedStateWriter(vertex.State)
	}

	// Process incoming messages from previous superstep (distributed mode only).
	// In BSP model, updates sent in superstep N are received in superstep N+1.
	// This enables distributed execution by deserializing state updates from the message bus.
	//
	// IMPORTANT: In single-process execution with shared StateManager, updates are applied
	// immediately after node execution (see below), so we skip incoming message processing
	// to avoid double-application. We detect shared state by checking if vertex.State
	// is the same reference as the global stateManager.
	//
	// TODO: Add explicit distributed mode flag instead of this heuristic
	isDistributed := n.runtime != nil && n.runtime.cg != nil &&
		n.runtime.cg.stateManager != nil && vertex.State != n.runtime.cg.stateManager

	if isDistributed && len(incoming) > 0 && vertex.State != nil {
		for _, msg := range incoming {
			payload := msg.Data
			if len(payload.Updates) > 0 || len(payload.Messages) > 0 {
				vertex.State.ApplyUpdates(payload.Updates, payload.Messages)
			}
		}
	}

	// Execute with retry policy if configured
	result, err := n.executeWithRetry(nodeCtx, bufferedState)

	if err != nil {
		if errors.Is(err, ErrHumanInterrupt) {
			logger.Info("node paused for human input",
				"node", n.name,
				"superstep", n.runtime.engine.CurrentSuperstep())
			n.runtime.cg.markPaused(n.name)
			n.runtime.setPaused(n.name)
			n.runtime.emit(stateif.ExecutionResult{Node: n.name, Err: ErrHumanInterrupt})
			return nil
		}
		logger.Error("node execution failed",
			"node", n.name,
			"superstep", n.runtime.engine.CurrentSuperstep(),
			"duration_ms", time.Since(startTime).Milliseconds(),
			"error", err)
		n.runtime.emit(stateif.ExecutionResult{Node: n.name, Err: err})
		return &NodeExecutionError{
			Node:      n.name,
			Superstep: n.runtime.engine.CurrentSuperstep(),
			Cause:     err,
		}
	}

	logger.Debug("node execution completed successfully",
		"node", n.name,
		"superstep", n.runtime.engine.CurrentSuperstep(),
		"duration_ms", time.Since(startTime).Milliseconds())

	var updates map[string]any
	var messages []stateif.ExecutionResult
	if result != nil {
		updates = result.Updates
		// Framework automatically wraps plain messages with execution metadata
		messages = n.wrapMessagesAsEvents(result.Messages)
	}

	// Flush buffered aggregates from the node execution
	if bufferedWriter, ok := bufferedState.(*bufferedStateWriter); ok {
		pendingAggregates := bufferedWriter.flushAggregates()
		if len(pendingAggregates) > 0 && vertex.State != nil {
			// Apply buffered aggregates to the actual state
			for name, values := range pendingAggregates {
				for _, value := range values {
					if err := vertex.State.RecordAggregation(name, value); err != nil {
						// Aggregator failures are terminal - they indicate state corruption
						aggErr := fmt.Errorf("node %q: aggregation %q failed: %w", n.name, name, err)
						logger.Error("aggregation failed",
							"node", n.name,
							"aggregator", name,
							"superstep", n.runtime.engine.CurrentSuperstep(),
							"error", err)
						n.runtime.emit(stateif.ExecutionResult{Node: n.name, Err: aggErr})
						return &NodeExecutionError{
							Node:      n.name,
							Superstep: n.runtime.engine.CurrentSuperstep(),
							Cause:     aggErr,
						}
					}
				}
			}
		}
	}

	// Emit one stateif.ExecutionResult per message
	for i := range messages {
		// Include Updates only in the first result
		if i == 0 {
			messages[i].Updates = updates
		}
		n.runtime.emit(messages[i])
	}

	// Hybrid approach for in-memory AND distributed execution:
	// Apply updates immediately to local state (for in-memory efficiency)
	// AND send them in messages (for distributed execution).
	// Downstream nodes check if updates are already applied to avoid double-application.
	if n.runtime != nil && n.runtime.cg != nil && n.runtime.cg.stateManager != nil {
		n.runtime.cg.stateManager.ApplyUpdates(updates, messages)
	}

	n.runtime.cg.clearPaused(n.name)
	n.runtime.cg.markCompleted(n.name)
	n.runtime.markExecuted(n.name)

	if err := n.handleScheduledDelivery(ctx, messages, updates); err != nil {
		return err
	}

	if n.runtime != nil && n.runtime.engine != nil {
		n.runtime.cg.setCurrentSuperstep(n.runtime.engine.CurrentSuperstep())
	}

	return nil
}

// executeWithRetry runs the node with retry logic if a RetryPolicy is configured.
//
//nolint:gocyclo // Retry logic with error handling requires multiple conditions
func (n *nodeAdapter) executeWithRetry(ctx context.Context, state stateif.Writer) (*NodeResult, error) {
	// Capture configured timeout duration at the start.
	// This ensures NodeTimeoutError.Timeout reflects the actual timeout budget,
	// not the time elapsed since timeout (which could be negative or zero).
	var timeoutMs int64
	if deadline, ok := ctx.Deadline(); ok {
		timeoutDuration := time.Until(deadline)
		if timeoutDuration <= 0 {
			return nil, &NodeTimeoutError{
				Node:    n.name,
				Timeout: 0,
				Cause:   context.DeadlineExceeded,
			}
		}
		timeoutMs = int64(timeoutDuration / time.Millisecond)
	}

	policy := n.node.RetryPolicy
	if policy == nil || policy.MaxAttempts <= 1 {
		// No retry policy or only single attempt
		result, err := n.node.Run(ctx, state)

		// Check if timeout occurred - always wrap DeadlineExceeded
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return nil, &NodeTimeoutError{
				Node:    n.name,
				Timeout: timeoutMs,
				Cause:   err,
			}
		}

		return result, err
	}

	backoffFn := policy.Backoff
	if backoffFn == nil {
		backoffFn = DefaultBackoff
	}

	retryableFn := policy.Retryable
	if retryableFn == nil {
		// All errors are retryable by default
		retryableFn = func(error) bool { return true }
	}

	var attempts []error
	var bufferedWriter *bufferedStateWriter
	if bw, ok := state.(*bufferedStateWriter); ok {
		bufferedWriter = bw
	}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if bufferedWriter != nil {
			bufferedWriter.resetAggregates()
		}

		result, err := n.node.Run(ctx, state)

		// Check if timeout occurred
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return nil, &NodeTimeoutError{
				Node:    n.name,
				Timeout: timeoutMs,
				Cause:   err,
			}
		}

		if err == nil {
			return result, nil
		}

		// Track all attempts with their errors
		attempts = append(attempts, fmt.Errorf("attempt %d: %w", attempt, err))

		// Don't retry if error is not retryable or if we're out of attempts
		if !retryableFn(err) || attempt >= policy.MaxAttempts {
			break
		}

		// Call OnRetry hook before sleeping
		// Check context before sleeping
		if err := ctx.Err(); err != nil {
			// Wrap timeout errors
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, &NodeTimeoutError{
					Node:    n.name,
					Timeout: timeoutMs,
					Cause:   err,
				}
			}
			return nil, err
		}

		// Wait before next attempt
		backoff := backoffFn(attempt)
		if backoff > 0 {
			// Cap backoff at 60 seconds to prevent excessive delays
			const MaxRetryBackoff = 60 * time.Second
			if backoff > MaxRetryBackoff {
				requestedBackoff := backoff
				backoff = MaxRetryBackoff
				n.runtime.emit(stateif.ExecutionResult{
					Node: n.name,
					Err:  fmt.Errorf("retry backoff capped at %v (requested %v)", MaxRetryBackoff, requestedBackoff),
				})
			}

			select {
			case <-ctx.Done():
				err := ctx.Err()
				// Wrap timeout errors
				if errors.Is(err, context.DeadlineExceeded) {
					return nil, &NodeTimeoutError{
						Node:    n.name,
						Timeout: timeoutMs,
						Cause:   err,
					}
				}
				return nil, err
			case <-time.After(backoff):
				// Continue to next attempt
			}
		}
	}

	// Check if we exited the loop due to timeout
	// This handles the case where the context times out between retry attempts
	// (e.g., during backoff sleep or between iterations) rather than during
	// node execution. Without this check, we would return RetryExhaustedError
	// instead of the more accurate NodeTimeoutError.
	if err := ctx.Err(); err != nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, &NodeTimeoutError{
			Node:    n.name,
			Timeout: timeoutMs,
			Cause:   err,
		}
	}

	return nil, &RetryExhaustedError{
		Node:     n.name,
		Attempts: attempts,
	}
}

// adaptCombiner converts a graph.Combiner to a pregel.Combiner[ChannelMessage].
// The combiner handles both routing metadata (From/To) and actual data payloads
// (Messages, Updates). This allows reducing mailbox pressure by merging multiple
// messages destined for the same node.
//
// The adapter:
// 1. Calls the user's combiner function on routing metadata
// 2. Merges the ChannelMessage data (messages and updates)
// 3. Returns a combined Pregel message
func adaptCombiner(fn Combiner) pregel.Combiner[ChannelMessage] {
	if fn == nil {
		return nil
	}
	return func(existing, incoming pregel.Message[ChannelMessage]) pregel.Message[ChannelMessage] {
		// Combine using the user's combiner function on the routing metadata
		combined := fn(
			SchedulingMessage{From: existing.From, To: existing.To},
			SchedulingMessage{From: incoming.From, To: incoming.To},
		)

		// Merge the channel message data
		mergedMsg := existing.Data
		mergedMsg.Messages = append(mergedMsg.Messages, incoming.Data.Messages...)
		if mergedMsg.Updates == nil {
			mergedMsg.Updates = make(map[string]any)
		}
		maps.Copy(mergedMsg.Updates, incoming.Data.Updates)

		return pregel.Message[ChannelMessage]{
			From: combined.From,
			To:   combined.To,
			Data: mergedMsg,
		}
	}
}

// StreamWriter is a function that can emit node results during execution.
// This is used for streaming node outputs in real-time rather than
// waiting for the entire graph execution to complete.
type StreamWriter func(*NodeResult)

var streamWriterContextKey = &struct{}{}

// withStreamWriter attaches a StreamWriter to a context.
// This allows nodes to emit results during execution via GetStreamWriter.
func withStreamWriter(ctx context.Context, writer StreamWriter) context.Context {
	return context.WithValue(ctx, streamWriterContextKey, writer)
}

// GetStreamWriter retrieves the StreamWriter from a context if present.
// Nodes can use this to emit results in real-time during execution.
// Returns nil if no StreamWriter is attached to the context.
func GetStreamWriter(ctx context.Context) StreamWriter {
	writer, _ := ctx.Value(streamWriterContextKey).(StreamWriter)
	return writer
}
