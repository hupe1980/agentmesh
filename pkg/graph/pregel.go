package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	ipregel "github.com/hupe1980/agentmesh/internal/pregel"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// =============================================================================
// ChannelMessage - Data payload for Pregel BSP
// =============================================================================

// ChannelMessage is the data-carrying message payload for Pregel BSP execution.
// It contains actual data to be communicated between nodes via channels.
type ChannelMessage struct {
	// Messages contains conversation messages to be passed between nodes
	Messages []message.Message `json:"messages,omitempty"`

	// Updates contains key-value state updates to be applied to channels
	Updates map[string]any `json:"updates,omitempty"`

	// Metadata contains additional routing or processing hints
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewChannelMessage creates a new channel message with the given messages and updates.
func NewChannelMessage(messages []message.Message, updates map[string]any) ChannelMessage {
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
		clone.Messages = make([]message.Message, len(cm.Messages))
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
// This separation allows the pure BSP engine (internal/pregel) to remain
// domain-agnostic while graph-specific concerns (channels, checkpoints,
// conditional routing) are handled at the graph layer.
type graphRuntime struct {
	cg      *CompiledGraph
	ctx     context.Context
	cancel  context.CancelFunc
	options runOptions
	stream  chan<- StreamEvent

	scheduler *vertexScheduler                               // Graph topology & routing
	engine    *ipregel.Runtime[StateManager, ChannelMessage] // BSP execution engine

	errOnce         sync.Once
	checkpointQueue chan *Checkpoint
	checkpointWG    sync.WaitGroup
}

// compiledPregelGraph adapts CompiledGraph to the pregel.PregelGraph interface.
// This allows the Pregel runtime to execute graph nodes without knowing about
// agent-specific concepts like channels, checkpoints, or conditional routing.
//
// The adapter pattern is used here to bridge between:
//   - Graph domain (StateManager, ChannelMessage, Node)
//   - Pregel domain (PregelGraph[S, M], PregelNode[S, M], Message[M])
type compiledPregelGraph struct {
	runtime *graphRuntime
}

// nodeAdapter wraps a graph.Node as a pregel.PregelNode.
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

func newPregelRuntime(cg *CompiledGraph, ctx context.Context, cancel context.CancelFunc, options runOptions, stream chan<- StreamEvent, done <-chan struct{}) *graphRuntime {
	scheduler := newVertexScheduler(cg)

	gr := &graphRuntime{
		cg:        cg,
		ctx:       ctx,
		cancel:    cancel,
		options:   options,
		stream:    stream,
		scheduler: scheduler,
	}

	// Note: maxMessages is now configured at StateManager creation time via NewStateManager(maxMessages).
	// The message limit cannot be changed after the state is created.

	adapter := &compiledPregelGraph{runtime: gr}
	maxWorkers := options.maxConcurrency
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	if cg != nil {
		cg.setCurrentSuperstep(options.initialSuperstep)
	}
	runtimeOptions := []ipregel.RuntimeOption[StateManager, ChannelMessage]{
		ipregel.WithMaxWorkers[StateManager, ChannelMessage](maxWorkers),
		ipregel.WithInitialSuperstep[StateManager, ChannelMessage](options.initialSuperstep),
	}
	if options.maxIterations > 0 {
		runtimeOptions = append(runtimeOptions, ipregel.WithMaxIterations[StateManager, ChannelMessage](options.maxIterations))
	}
	if len(options.aggregators) > 0 {
		runtimeOptions = append(runtimeOptions, ipregel.WithAggregators[StateManager, ChannelMessage](adaptAggregators(options.aggregators)))
	}
	if options.combiner != nil {
		runtimeOptions = append(runtimeOptions, ipregel.WithCombiner[StateManager, ChannelMessage](adaptCombiner(options.combiner)))
	}
	// Install checkpoint callback if configured
	if options.checkpointer != nil && options.runID != "" && options.checkpointInterval > 0 {
		runtimeOptions = append(runtimeOptions, ipregel.WithOnSuperstepComplete[StateManager, ChannelMessage](func(superstep int64) {
			gr.saveCheckpoint(superstep)
		}))
	}

	// Create the Pregel runtime (use MustNewRuntime since inputs are already validated)
	gr.engine = ipregel.MustNewRuntime(adapter, nil, runtimeOptions...)

	// Configure the engine to respect early termination
	if done != nil {
		gr.engine.SetDoneChannel(done)
	}

	gr.startCheckpointWorker()

	return gr
}

func (gr *graphRuntime) startCheckpointWorker() {
	if gr.options.checkpointer == nil || gr.options.runID == "" {
		return
	}
	if gr.checkpointQueue != nil {
		return
	}

	gr.checkpointQueue = make(chan *Checkpoint, 1)
	saveCtx := context.WithoutCancel(gr.ctx)
	gr.checkpointWG.Add(1)

	go func() {
		defer gr.checkpointWG.Done()
		for checkpoint := range gr.checkpointQueue {
			if checkpoint == nil {
				continue
			}
			if err := gr.options.checkpointer.Save(saveCtx, checkpoint); err != nil {
				gr.emitError(fmt.Errorf("failed to save checkpoint at superstep %d: %w", checkpoint.Superstep, err))
			}
		}
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

func (gr *graphRuntime) run() error {
	defer gr.stopCheckpointWorker()

	if gr.cg != nil {
		gr.cg.bootstrapScheduler(gr.ctx, gr.scheduler)
	}

	err := gr.engine.Run(gr.ctx)
	if gr.cg != nil {
		gr.cg.setCurrentSuperstep(gr.engine.Stats().Supersteps)
	}

	// Transfer final aggregates to graph state
	if err == nil || errors.Is(err, context.Canceled) {
		if aggregates := gr.engine.Aggregates(); len(aggregates) > 0 {
			gr.cg.stateManager.SetAggregates(aggregates)
		}
	}

	// The Pregel engine can return context.DeadlineExceeded directly from ctx.Err().
	// Wrap it here to ensure consistent error types. Node-level timeouts are already
	// wrapped by the node adapter, but runtime-level timeouts need wrapping here.
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		// Check if already wrapped (node-level timeout)
		var nodeTimeoutErr *NodeTimeoutError
		if !errors.As(err, &nodeTimeoutErr) {
			// Not wrapped yet - this is a runtime-level timeout
			timeout := int64(0)
			if deadline, ok := gr.ctx.Deadline(); ok {
				elapsed := time.Since(deadline)
				if elapsed > 0 {
					timeout = int64(elapsed / time.Millisecond)
				}
			}
			err = &NodeTimeoutError{
				Node:    "", // Runtime-level timeout (not node-specific)
				Timeout: timeout,
				Cause:   err,
			}
		}
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		gr.fail(err)
	}
	return err
}

func (gr *graphRuntime) saveCheckpoint(superstep int64) {
	// Skip checkpoint if not configured or interval not reached
	if gr.options.checkpointer == nil || gr.options.runID == "" {
		return
	}
	if gr.options.checkpointInterval > 0 && superstep%int64(gr.options.checkpointInterval) != 0 {
		return
	}

	// Create checkpoint from current state
	checkpoint := gr.cg.createCheckpoint(gr.options.runID, superstep, nil)
	if checkpoint == nil {
		return
	}

	if gr.checkpointQueue != nil {
		select {
		case gr.checkpointQueue <- checkpoint:
		default:
			gr.emitError(fmt.Errorf("checkpoint queue full at superstep %d: dropping checkpoint", superstep))
		}
		return
	}

	if err := gr.options.checkpointer.Save(context.WithoutCancel(gr.ctx), checkpoint); err != nil {
		gr.emitError(fmt.Errorf("failed to save checkpoint at superstep %d: %w", superstep, err))
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

func (gr *graphRuntime) emit(event StreamEvent) {
	select {
	case <-gr.ctx.Done():
	case gr.stream <- event:
	}
}

func (gr *graphRuntime) fail(err error) {
	if err == nil {
		return
	}
	gr.errOnce.Do(func() {
		gr.emit(StreamEvent{Err: err})
		if gr.cancel != nil {
			gr.cancel()
		}
	})
}

func (gr *graphRuntime) emitError(err error) {
	if err == nil {
		return
	}
	gr.emit(StreamEvent{Err: err})
}

// compiledPregelGraph implements the internal pregel interfaces for CompiledGraph.

func (g *compiledPregelGraph) RootNodes() []string {
	return g.runtime.scheduler.Ready()
}

func (g *compiledPregelGraph) Outgoing(node string) []string {
	if targets := g.runtime.cg.outgoing[node]; len(targets) > 0 {
		return append([]string(nil), targets...)
	}
	return nil
}

func (g *compiledPregelGraph) NodeByName(name string) ipregel.PregelNode[StateManager, ChannelMessage] {
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

func (g *compiledPregelGraph) Update(string, map[string]any, []ipregel.Message[ChannelMessage]) {
	// No-op: channel messages are handled explicitly by node adapters.
}

// nodeAdapter executes both standard and command-style nodes within the Pregel runtime.

func (n *nodeAdapter) Name() string { return n.name }

//nolint:gocyclo // Node execution requires handling many runtime conditions
func (n *nodeAdapter) Run(ctx context.Context, vertex ipregel.VertexContext[StateManager, ChannelMessage], incoming []ipregel.Message[ChannelMessage]) error {
	writer := func(result *NodeResult) {
		if result == nil {
			return
		}
		n.runtime.emit(StreamEvent{Node: n.name, Result: cloneNodeResult(result)})
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

	// Execute with retry policy if configured
	result, err := n.executeWithRetry(nodeCtx, bufferedState)

	if err != nil {
		if errors.Is(err, ErrHumanInterrupt) {
			n.runtime.cg.markPaused(n.name)
			n.runtime.setPaused(n.name)
			n.runtime.emit(StreamEvent{Node: n.name, Err: ErrHumanInterrupt})
			return nil
		}
		n.runtime.emit(StreamEvent{Node: n.name, Err: err})
		return &NodeExecutionError{
			Node:      n.name,
			Superstep: n.runtime.engine.CurrentSuperstep(),
			Cause:     err,
		}
	}

	var updates map[string]any
	var messages []message.Message
	if result != nil {
		updates = result.Updates
		messages = result.Messages
	}

	// Flush buffered aggregates from the node execution
	if bufferedWriter, ok := bufferedState.(*bufferedStateWriter); ok {
		pendingAggregates := bufferedWriter.flushAggregates()
		if len(pendingAggregates) > 0 && vertex.State != nil {
			// Apply buffered aggregates to the actual state
			for name, values := range pendingAggregates {
				for _, value := range values {
					if err := vertex.State.RecordAggregation(name, value); err != nil {
						// Log error but don't fail the node
						n.runtime.emit(StreamEvent{Node: n.name, Err: fmt.Errorf("aggregate %q failed: %w", name, err)})
					}
				}
			}
		}
	}

	event := StreamEvent{Node: n.name, Updates: updates, Messages: cloneMessages(messages)}
	n.runtime.emit(event)

	if n.runtime != nil && n.runtime.cg != nil && n.runtime.cg.stateManager != nil {
		n.runtime.cg.stateManager.ApplyUpdates(updates, messages)
	}

	n.runtime.cg.clearPaused(n.name)
	n.runtime.cg.markCompleted(n.name)
	n.runtime.markExecuted(n.name)

	if n.runtime != nil && n.runtime.scheduler != nil {
		next, schedErr := n.runtime.onVertexCompleted(ctx, n.name)
		if schedErr != nil {
			return schedErr
		}
		if len(next) > 0 && n.runtime.engine != nil {
			// Create channel messages with data from node execution
			deliveries := make([]ipregel.Message[ChannelMessage], 0, len(next))
			for _, target := range next {
				// Send actual data in the message (not empty signal)
				msg := NewChannelMessage(messages, updates)
				deliveries = append(deliveries, ipregel.Message[ChannelMessage]{
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
		}
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
	// Check if context has a deadline and enforce timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return nil, &NodeTimeoutError{
				Node:    n.name,
				Timeout: 0,
				Cause:   context.DeadlineExceeded,
			}
		}
	}

	policy := n.node.RetryPolicy
	if policy == nil || policy.MaxAttempts <= 1 {
		// No retry policy or only single attempt
		result, err := n.node.Run(ctx, state)

		// Check if timeout occurred - always wrap DeadlineExceeded
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			timeout := int64(0)
			if deadline, ok := ctx.Deadline(); ok {
				elapsed := time.Since(deadline)
				if elapsed > 0 {
					timeout = int64(elapsed / time.Millisecond)
				}
			}
			return nil, &NodeTimeoutError{
				Node:    n.name,
				Timeout: timeout,
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
			if deadline, ok := ctx.Deadline(); ok {
				return nil, &NodeTimeoutError{
					Node:    n.name,
					Timeout: int64(time.Since(deadline.Add(-time.Until(deadline))) / time.Millisecond),
					Cause:   err,
				}
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
				timeout := int64(0)
				if deadline, ok := ctx.Deadline(); ok {
					elapsed := time.Since(deadline)
					if elapsed > 0 {
						timeout = int64(elapsed / time.Millisecond)
					}
				}
				return nil, &NodeTimeoutError{
					Node:    n.name,
					Timeout: timeout,
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
				n.runtime.emit(StreamEvent{
					Node: n.name,
					Err:  fmt.Errorf("retry backoff capped at %v (requested %v)", MaxRetryBackoff, requestedBackoff),
				})
			}

			select {
			case <-ctx.Done():
				err := ctx.Err()
				// Wrap timeout errors
				if errors.Is(err, context.DeadlineExceeded) {
					timeout := int64(0)
					if deadline, ok := ctx.Deadline(); ok {
						elapsed := time.Since(deadline)
						if elapsed > 0 {
							timeout = int64(elapsed / time.Millisecond)
						}
					}
					return nil, &NodeTimeoutError{
						Node:    n.name,
						Timeout: timeout,
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
		timeout := int64(0)
		if deadline, ok := ctx.Deadline(); ok {
			elapsed := time.Since(deadline)
			if elapsed > 0 {
				timeout = int64(elapsed / time.Millisecond)
			}
		}
		return nil, &NodeTimeoutError{
			Node:    n.name,
			Timeout: timeout,
			Cause:   err,
		}
	}

	return nil, &RetryExhaustedError{
		Node:     n.name,
		Attempts: attempts,
	}
}

// adaptAggregators converts graph.Aggregator to pregel.Aggregator.
// This adapter allows graph-level aggregators (with domain-specific
// implementations like SumAggregator, AvgAggregator) to work with the
// generic Pregel runtime.
//
// The interfaces are identical but defined in separate packages to maintain
// package independence. This adapter is zero-cost (interface wrapper only).
func adaptAggregators(source map[string]Aggregator) map[string]ipregel.Aggregator {
	if len(source) == 0 {
		return nil
	}
	mapped := make(map[string]ipregel.Aggregator, len(source))
	for name, agg := range source {
		if name == "" || agg == nil {
			continue
		}
		mapped[name] = aggregatorAdapter{agg: agg}
	}
	if len(mapped) == 0 {
		return nil
	}
	return mapped
}

// aggregatorAdapter implements pregel.Aggregator by delegating to graph.Aggregator.
// This is a simple wrapper that enables graph-level aggregators to work with
// the Pregel runtime without any modifications.
type aggregatorAdapter struct {
	agg Aggregator
}

func (a aggregatorAdapter) Zero() any {
	return a.agg.Zero()
}

func (a aggregatorAdapter) Aggregate(current, value any) any {
	return a.agg.Aggregate(current, value)
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
func adaptCombiner(fn Combiner) ipregel.Combiner[ChannelMessage] {
	if fn == nil {
		return nil
	}
	return func(existing, incoming ipregel.Message[ChannelMessage]) ipregel.Message[ChannelMessage] {
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

		return ipregel.Message[ChannelMessage]{
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

func cloneNodeResult(result *NodeResult) *NodeResult {
	if result == nil {
		return nil
	}
	var updates map[string]any
	if len(result.Updates) > 0 {
		updates = make(map[string]any, len(result.Updates))
		for k, v := range result.Updates {
			updates[k] = v
		}
	}
	return &NodeResult{
		Updates:  updates,
		Messages: cloneMessages(result.Messages),
	}
}
