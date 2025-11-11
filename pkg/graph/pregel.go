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
	Messages []Event `json:"messages,omitzero"`

	// Updates contains key-value state updates to be applied to channels
	Updates map[string]any `json:"updates,omitzero"`

	// Metadata contains additional routing or processing hints
	Metadata map[string]string `json:"metadata,omitzero"`
}

// NewChannelMessage creates a new channel message with the given message events and updates.
func NewChannelMessage(messages []Event, updates map[string]any) ChannelMessage {
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
		clone.Messages = make([]Event, len(cm.Messages))
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
// The yield function is used to emit events directly to the iterator consumer.
// The cancel function is stored for early termination when yield returns false.
type graphRuntime struct {
	cg      *Compiled
	cancel  context.CancelFunc
	options runOptions
	yield   func(Event, error) bool // Iterator yield function for emitting events

	scheduler       *vertexScheduler                              // Graph topology & routing
	engine          *pregel.Runtime[StateManager, ChannelMessage] // BSP execution engine
	instrumentation *Instrumentation                              // Observability instrumentation (passed from options)

	errOnce         sync.Once
	yieldMu         sync.Mutex // Protects yield from concurrent access
	yieldStopped    bool       // True when yield has returned false
	checkpointQueue chan *checkpoint.Checkpoint
	checkpointWG    sync.WaitGroup
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

// wrapMessagesAsEvents wraps raw messages from node execution in Event structs
// with execution metadata (graphID, nodeName, timestamp, UUID).
func (n *nodeAdapter) wrapMessagesAsEvents(messages []message.Message) []Event {
	if len(messages) == 0 {
		return nil
	}

	graphID := ""
	if n.runtime != nil {
		graphID = n.runtime.options.runID
	}

	events := make([]Event, len(messages))
	for i, msg := range messages {
		events[i] = *NewEvent(msg, graphID, n.name)
	}

	return events
}

func newPregelRuntime(cg *Compiled, cancel context.CancelFunc, options runOptions, yield func(Event, error) bool, instrumentation *Instrumentation) *graphRuntime {
	scheduler := newVertexScheduler(cg)

	gr := &graphRuntime{
		cg:              cg,
		cancel:          cancel,
		options:         options,
		yield:           yield,
		scheduler:       scheduler,
		instrumentation: instrumentation,
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
	logger := logging.FromContext(ctx)

	if gr.options.checkpointer == nil || gr.options.runID == "" {
		return
	}
	if gr.checkpointQueue != nil {
		return
	}

	logger.Debug("starting async checkpoint worker",
		"run_id", gr.options.runID,
		"queue_size", 1)

	gr.checkpointQueue = make(chan *checkpoint.Checkpoint, 1)
	// Create detached context for background saves (checkpoints must complete even if request canceled)
	saveCtx := context.WithoutCancel(ctx)
	gr.checkpointWG.Add(1)

	go func() {
		defer gr.checkpointWG.Done()
		logger := logging.FromContext(saveCtx)

		for checkpoint := range gr.checkpointQueue {
			if checkpoint == nil {
				continue
			}

			logger.Debug("processing checkpoint from queue",
				"run_id", checkpoint.RunID,
				"superstep", checkpoint.Superstep)

			if err := gr.options.checkpointer.Save(saveCtx, checkpoint); err != nil {
				checkpointErr := fmt.Errorf("failed to save checkpoint at superstep %d: %w", checkpoint.Superstep, err)
				logger.Error("async checkpoint save failed",
					"run_id", checkpoint.RunID,
					"superstep", checkpoint.Superstep,
					"error", err)
				if gr.options.failOnCheckpointError {
					gr.fail(checkpointErr)
				} else {
					gr.emitError(checkpointErr)
				}
			} else {
				logger.Info("async checkpoint saved successfully",
					"run_id", checkpoint.RunID,
					"superstep", checkpoint.Superstep,
					"version", checkpoint.Version)
			}
		}

		logger.Debug("checkpoint worker stopped", "run_id", gr.options.runID)
	}()
}

func (gr *graphRuntime) stopCheckpointWorker() {
	if gr.checkpointQueue == nil {
		return
	}
	close(gr.checkpointQueue)
	gr.checkpointWG.Wait()
	gr.checkpointQueue = nil
}

func (gr *graphRuntime) run(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	startTime := time.Now()

	logger.Info("starting graph execution",
		"run_id", gr.options.runID,
		"checkpoint_interval", gr.options.checkpointInterval)

	// Start checkpoint worker with this request's context
	gr.startCheckpointWorker(ctx)
	defer gr.stopCheckpointWorker()

	// Start runtime trace span
	var span trace.Span
	if gr.instrumentation != nil {
		ctx, span = gr.instrumentation.TraceGraphExecution(ctx, "runtime.run")
		defer span.End(nil)
	}

	if gr.cg != nil {
		gr.cg.bootstrapScheduler(ctx, gr.scheduler)
	}

	err := gr.engine.Run(ctx)
	if gr.cg != nil {
		gr.cg.setCurrentSuperstep(gr.engine.Stats().Supersteps)
	}

	duration := time.Since(startTime)

	// Transfer final aggregates to graph state
	if err == nil || errors.Is(err, context.Canceled) {
		if aggregates := gr.engine.Aggregates(); len(aggregates) > 0 {
			gr.cg.stateManager.SetAggregates(aggregates)
		}
	}

	// The Pregel engine can return context.DeadlineExceeded directly from ctx.Err().
	// Wrap it here to ensure consistent error types. Node-level timeouts are already
	// wrapped by the node adapter, but runtime-level timeouts need wrapping here.
	err = wrapTimeoutError(ctx, err)

	switch {
	case err != nil && !errors.Is(err, context.Canceled):
		logger.Error("graph execution failed",
			"run_id", gr.options.runID,
			"supersteps", gr.engine.Stats().Supersteps,
			"duration_ms", duration.Milliseconds(),
			"error", err)
		gr.fail(err)
	case errors.Is(err, context.Canceled):
		logger.Warn("graph execution canceled",
			"run_id", gr.options.runID,
			"supersteps", gr.engine.Stats().Supersteps,
			"duration_ms", duration.Milliseconds())
	default:
		logger.Info("graph execution completed successfully",
			"run_id", gr.options.runID,
			"supersteps", gr.engine.Stats().Supersteps,
			"duration_ms", duration.Milliseconds())
	}
	return err
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
	var span trace.Span
	if gr.instrumentation != nil {
		ctx, span = gr.instrumentation.TraceCheckpoint(ctx, "save", gr.options.runID, superstep)
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

	if gr.checkpointQueue != nil {
		select {
		case gr.checkpointQueue <- checkpoint:
			logger.Debug("checkpoint queued for async save",
				"run_id", gr.options.runID,
				"superstep", superstep)
		default:
			// Queue is full - checkpoint worker is busy processing previous save
			// This is expected under high checkpoint frequency. Try again with timeout
			// to avoid dropping important checkpoints while still respecting context cancellation.
			logger.Warn("checkpoint queue full, waiting for worker",
				"run_id", gr.options.runID,
				"superstep", superstep)

			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()

			select {
			case gr.checkpointQueue <- checkpoint:
				logger.Debug("checkpoint queued after wait",
					"run_id", gr.options.runID,
					"superstep", superstep)
			case <-timer.C:
				queueErr := fmt.Errorf("checkpoint queue timeout at superstep %d after 5s: checkpoint dropped", superstep)
				logger.Error("checkpoint queue timeout",
					"run_id", gr.options.runID,
					"superstep", superstep)
				if gr.options.failOnCheckpointError {
					gr.fail(queueErr)
				} else {
					gr.emitError(queueErr)
				}
			case <-ctx.Done():
				logger.Warn("checkpoint save cancelled",
					"run_id", gr.options.runID,
					"superstep", superstep)
			}
		}
		return
	}

	if err := gr.options.checkpointer.Save(context.WithoutCancel(ctx), checkpoint); err != nil {
		checkpointErr := fmt.Errorf("failed to save checkpoint at superstep %d: %w", superstep, err)
		logger.Error("failed to save checkpoint",
			"run_id", gr.options.runID,
			"superstep", superstep,
			"error", err)
		if gr.options.failOnCheckpointError {
			gr.fail(checkpointErr)
		} else {
			gr.emitError(checkpointErr)
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

func (gr *graphRuntime) emit(event Event) {
	if gr.yield == nil {
		return
	}

	gr.yieldMu.Lock()
	defer gr.yieldMu.Unlock()

	// Don't call yield if it already returned false
	if gr.yieldStopped {
		return
	}

	// Call yield and mark as stopped if it returns false
	if !gr.yield(event, event.Err) {
		gr.yieldStopped = true
		if gr.cancel != nil {
			gr.cancel()
		}
	}
}

func (gr *graphRuntime) fail(err error) {
	if err == nil {
		return
	}
	gr.errOnce.Do(func() {
		gr.emit(Event{Err: err})
		if gr.cancel != nil {
			gr.cancel()
		}
	})
}

func (gr *graphRuntime) emitError(err error) {
	if err == nil {
		return
	}
	gr.emit(Event{Err: err})
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
func (n *nodeAdapter) handleScheduledDelivery(ctx context.Context, messages []Event, updates map[string]any) error {
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
	if n.runtime.instrumentation != nil {
		ctx, span = n.runtime.instrumentation.TraceNodeExecution(ctx, n.name, superstep)
	}
	defer func() {
		// Record metrics and end span
		duration := time.Since(startTime)
		if n.runtime.instrumentation != nil {
			n.runtime.instrumentation.RecordNodeExecution(ctx, n.name, duration, nil)
			span.End(nil)
		}
	}()

	writer := func(result *NodeResult) {
		if result == nil {
			return
		}
		// Emit one Event per message
		if len(result.Messages) == 0 {
			// No messages: emit a single event with just Updates
			n.runtime.emit(Event{
				Node:    n.name,
				Updates: result.Updates,
			})
		} else {
			for i, msg := range result.Messages {
				evt := NewEvent(msg, n.runtime.options.runID, n.name)
				// Include Updates only in the first event
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
	var bufferedState StateWriter
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
			n.runtime.emit(Event{Node: n.name, Err: ErrHumanInterrupt})
			return nil
		}
		logger.Error("node execution failed",
			"node", n.name,
			"superstep", n.runtime.engine.CurrentSuperstep(),
			"duration_ms", time.Since(startTime).Milliseconds(),
			"error", err)
		n.runtime.emit(Event{Node: n.name, Err: err})
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
	var messages []Event
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
						n.runtime.emit(Event{Node: n.name, Err: aggErr})
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

	// Emit one Event per message
	for i := range messages {
		// Include Updates only in the first event
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
func (n *nodeAdapter) executeWithRetry(ctx context.Context, state StateWriter) (*NodeResult, error) {
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
				n.runtime.emit(Event{
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

