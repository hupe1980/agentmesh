package exec

import (
	"context"
	"fmt"
	"iter"
	"runtime"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Pregel is a Bulk-Synchronous Parallel (BSP) executor that uses the pregel runtime.
type Pregel struct {
	maxWorkers             int
	maxIters               int
	messageBus             pregel.MessageBus[message.Message]
	aggregators            map[string]pregel.Aggregator
	enableDistributedState bool            // Enable state synchronization via message bus
	metrics                *RuntimeMetrics // Track execution metadata for checkpoints
}

// PregelOption configures a Pregel executor.
type PregelOption func(*Pregel)

// WithMaxWorkers sets the maximum number of parallel workers.
func WithMaxWorkers(n int) PregelOption {
	return func(p *Pregel) {
		if n > 0 {
			p.maxWorkers = n
		}
	}
}

// WithMaxIterations sets the maximum number of supersteps.
func WithMaxIterations(n int) PregelOption {
	return func(p *Pregel) {
		p.maxIters = n
	}
}

// WithMessageBus sets a custom message bus for graph execution.
//
// Message buses can be:
//   - In-memory (default): pregel.NewInMemoryMessageBus() - local execution
//   - Redis: predis.NewMessageBus() - distributed across machines
//
// The Pregel executor behaves identically regardless of message bus type.
// Distributed state synchronization is automatically enabled to ensure
// state updates propagate correctly through the message bus.
//
// To disable state sync (routing-only messages): use WithDistributedState(false)
func WithMessageBus(bus pregel.MessageBus[message.Message]) PregelOption {
	return func(p *Pregel) {
		p.messageBus = bus
		p.enableDistributedState = true // Auto-enable when message bus is provided
	}
}

// WithAggregators configures global aggregators.
func WithAggregators(aggs map[string]pregel.Aggregator) PregelOption {
	return func(p *Pregel) {
		p.aggregators = aggs
	}
}

// WithDistributedState controls distributed state synchronization via the message bus.
// Pass true to enable (default when message bus is set), false to disable.
//
// When enabled (default), state updates from each node are serialized and sent
// through the message bus (in-memory or Redis), allowing nodes to receive and
// apply state changes. This is required for state-based workflows.
//
// When disabled, only routing signals are sent through the message bus.
// This is lighter but state updates remain local-only.
//
// Use cases:
//   - State-based workflows (most graphs): Keep enabled (default)
//   - Pure BSP algorithms (PageRank, etc.): Can disable for efficiency
//
// Note: WithMessageBus() automatically enables this. Use WithDistributedState(false)
// to explicitly disable if you only need routing without state propagation.
func WithDistributedState(enable ...bool) PregelOption {
	enabled := true
	if len(enable) > 0 {
		enabled = enable[0]
	}
	return func(p *Pregel) {
		p.enableDistributedState = enabled
	}
}

// NewPregelExecutor creates a new Pregel BSP executor.
//
// The executor runs the same BSP algorithm regardless of configuration.
// Options control where messages go and what they contain:
//
//   - Default: In-memory message passing, no state sync (local execution)
//   - WithMessageBus(): Custom transport (in-memory or Redis), auto-enables state sync
//   - WithDistributedState(false): Disable state sync for routing-only messages
//
// The Pregel logic is identical whether messages are in-memory or distributed via Redis.
func NewPregelExecutor(opts ...PregelOption) *Pregel {
	p := &Pregel{
		maxWorkers: runtime.NumCPU(),
		maxIters:   1000,
		metrics:    NewRuntimeMetrics(), // Initialize execution tracking
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run executes the compiled graph using the pregel BSP runtime.
//
//nolint:gocyclo // Complex graph orchestration logic cannot be easily simplified
func (p *Pregel) Run(
	ctx context.Context,
	compiled *compile.CompiledGraph,
	input []message.Message,
	opts ...graph.RunOption) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		// Extract run options
		runOpts := extractRunOptions(opts)

		// Create a cancellable context so we can stop the runtime when consumer exits
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		runID := runOpts.RunID
		if runID == "" {
			runID = uuid.New().String()
		}

		// Restore from checkpoint if configured
		if err := p.restoreCheckpoint(runCtx, compiled, runOpts); err != nil {
			yield(nil, err)
			return
		}

		// Start checkpoint worker
		worker := p.startCheckpointWorker(runCtx, runOpts)
		defer p.stopCheckpointWorker(worker)

		// Store initial messages in state
		// Note: Uses "__messages__" key name (defined in agent.MessagesKey)
		if len(input) > 0 {
			updates := state.Updates{}
			updates["__messages__"] = input
			if err := state.ApplyUpdates(runCtx, compiled.Manager, updates); err != nil {
				yield(nil, fmt.Errorf("failed to store initial messages: %w", err))
				return
			}
		}

		// Create result channel to serialize all yields from a single goroutine
		resultChan := make(chan struct {
			msg message.Message
			err error
		}, 100)

		// Single goroutine that calls yield - ensures thread safety
		yieldDone := make(chan struct{})
		go func() {
			defer close(yieldDone)
			for item := range resultChan {
				if !yield(item.msg, item.err) {
					cancel() // Consumer stopped, cancel the runtime context
					// Drain remaining items to prevent goroutine leak
					//nolint:revive // Need to drain channel
					for range resultChan {
					}
					return
				}
			}
		}()

		// Helper to send results to the yield goroutine
		safeYield := func(msg message.Message, err error) bool {
			select {
			case resultChan <- struct {
				msg message.Message
				err error
			}{msg, err}:
				return true
			case <-runCtx.Done():
				return false
			}
		}

		// Create adapter to make compiled graph work with pregel runtime
		adapter := &pregelGraphAdapter{
			compiled:               compiled,
			runID:                  runID,
			yield:                  safeYield,
			enableDistributedState: p.enableDistributedState,
			executor:               p,
		}

		// Configure pregel runtime options
		runtimeOpts := []pregel.RuntimeOption[*compile.CompiledGraph, message.Message]{
			pregel.WithMaxWorkers[*compile.CompiledGraph, message.Message](p.maxWorkers),
			pregel.WithMaxIterations[*compile.CompiledGraph, message.Message](p.maxIters),
		}

		if p.messageBus != nil {
			runtimeOpts = append(runtimeOpts, pregel.WithMessageBus[*compile.CompiledGraph, message.Message](p.messageBus))
		}

		if len(p.aggregators) > 0 {
			runtimeOpts = append(runtimeOpts, pregel.WithAggregators[*compile.CompiledGraph, message.Message](p.aggregators))
		}

		// Add superstep completion callback for checkpointing
		if runOpts.Checkpointer != nil && runOpts.RunID != "" {
			runtimeOpts = append(runtimeOpts, pregel.WithOnSuperstepComplete[*compile.CompiledGraph, message.Message](
				func(ctx context.Context, superstep int64) {
					p.saveCheckpoint(ctx, compiled, runOpts, superstep, worker)
				},
			))
		}

		// Create and run the pregel runtime
		rt, err := pregel.NewRuntime[*compile.CompiledGraph, message.Message](adapter, runtimeOpts...)
		if err != nil {
			safeYield(nil, err)
			close(resultChan)
			<-yieldDone
			return
		}

		// Execute runtime and forward events to result channel
		for _, err := range rt.Run(runCtx) {
			// Check if context was cancelled (consumer stopped iterating)
			select {
			case <-runCtx.Done():
				// Stop immediately when context cancelled
				close(resultChan)
				<-yieldDone
				return
			default:
			}

			if err != nil {
				// Fatal error - BSP execution terminated
				// Yield error and stop iteration
				safeYield(nil, err)
				break
			}
		}

		close(resultChan)
		<-yieldDone // Wait for yield goroutine to finish
	}
}

// pregelGraphAdapter adapts a CompiledGraph to the pregel.Graph interface.
type pregelGraphAdapter struct {
	compiled               *compile.CompiledGraph
	runID                  string
	yield                  func(message.Message, error) bool
	enableDistributedState bool
	executor               *Pregel // Reference to executor for metrics tracking
}

// RootNodes returns nodes with no incoming edges.
func (a *pregelGraphAdapter) RootNodes() []string {
	var roots []string
	for _, nodeName := range a.compiled.Topology.NodeNames {
		if nodeName == a.compiled.StartNode || nodeName == a.compiled.EndNode {
			continue
		}
		if a.compiled.Topology.Incoming[nodeName] == 0 {
			roots = append(roots, nodeName)
		}
	}
	return roots
}

// Outgoing returns outgoing node names.
func (a *pregelGraphAdapter) Outgoing(nodeName string) []string {
	return a.compiled.Topology.Outgoing[nodeName]
}

// NodeByName returns a pregel node adapter.
func (a *pregelGraphAdapter) NodeByName(name string) pregel.Node[*compile.CompiledGraph, message.Message] {
	return &pregelNodeAdapter{
		nodeName:               name,
		compiled:               a.compiled,
		runID:                  a.runID,
		yield:                  a.yield,
		enableDistributedState: a.enableDistributedState,
		executor:               a.executor,
	}
}

// State returns the compiled graph (used as global state).
func (a *pregelGraphAdapter) State() *compile.CompiledGraph {
	return a.compiled
}

// pregelNodeAdapter adapts a graph.Node to pregel.Node interface.
type pregelNodeAdapter struct {
	nodeName               string
	compiled               *compile.CompiledGraph
	runID                  string
	yield                  func(message.Message, error) bool
	enableDistributedState bool
	executor               *Pregel // Reference to executor for metrics tracking
}

// Name returns the node name.
func (n *pregelNodeAdapter) Name() string {
	return n.nodeName
}

// Run executes the node.
//
//nolint:gocyclo,nestif // BSP node execution requires complex state management
func (n *pregelNodeAdapter) Run(
	ctx context.Context,
	vertex pregel.VertexContext[*compile.CompiledGraph, message.Message],
	incoming []pregel.Message[message.Message],
) error {
	node := n.compiled.GetNode(n.nodeName)
	if node == nil {
		return nil // Skip missing nodes
	}

	// Check if node is paused (human-in-the-loop scenario)
	if n.executor != nil && n.executor.metrics != nil {
		snapshot := n.executor.metrics.Snapshot()

		// Only skip paused nodes - they require external resume signal
		for _, pausedNode := range snapshot.PausedNodes {
			if pausedNode == n.nodeName {
				// Node is paused, wait for external ResumePaused call
				// CompletedNodes are tracked but don't prevent re-execution by default
				// This allows normal graph re-execution after checkpoint resume
				return nil
			}
		}
	}

	// If distributed state is enabled, apply incoming state updates first
	// TODO: Distributed state synchronization needs to be reimplemented for new state system
	// The new state system uses typed keys (Key[T]) and ApplyUpdates for batch mutations
	// Distributed state sync should:
	// 1. Receive state.Updates from remote nodes
	// 2. Call state.ApplyUpdates(ctx, compiled.Manager, updates) at BSP barriers
	// 3. Use state snapshots for consistent reads across distributed nodes
	if n.enableDistributedState {
		// Disabled until reimplemented for new state system
		_ = incoming
	}

	// Create a stream writer that yields intermediate results
	streamWriter := func(intermediateResult *graph.NodeResult) {
		if intermediateResult == nil {
			return
		}

		// Extract messages from updates and create execution events
		// Note: Uses "__messages__" key name (defined in agent.MessagesKey)
		if messagesAny, ok := intermediateResult.Updates["__messages__"]; ok {
			if messages, ok := messagesAny.([]message.Message); ok && len(messages) > 0 {
				// Yield each intermediate message directly
				for _, msg := range messages {
					n.yield(msg, nil)
				}
			}
		}
	}

	// Attach stream writer to context
	ctxWithStream := graph.WithStreamWriter(ctx, streamWriter)

	// Execute the node with BSP-correct state access:
	// 1. Create read-only snapshot view of current state (BSP read phase)
	// 2. Execute node with the snapshot (isolated from concurrent updates)
	// 3. Collect updates for batch application at superstep barrier (BSP write phase)
	view, err := n.compiled.Manager.CreateReadView(ctx)
	if err != nil {
		return fmt.Errorf("failed to create read view: %w", err)
	}
	result, err := node.Run(ctxWithStream, view)
	if err != nil {
		// Wrap node execution errors with sentinel for identification
		return fmt.Errorf("%w: node %q: %v", state.ErrNodeExecution, n.nodeName, err)
	}

	if result == nil {
		return nil
	}

	// Track node completion for checkpoint metadata
	if n.executor != nil && n.executor.metrics != nil {
		n.executor.metrics.AddCompleted(n.nodeName)
	}

	// Apply updates immediately after node execution
	// This is necessary for routing decisions (conditional edges) that evaluate
	// state right after the node completes. In BSP terminology, this is still
	// correct because:
	// 1. Nodes in the same superstep run in parallel (no intra-superstep dependencies)
	// 2. Routing/messaging happens after compute phase (between supersteps)
	// 3. Each node sees a consistent snapshot at superstep start (via ReadView)
	if len(result.Updates) > 0 {
		if err := state.ApplyUpdates(ctx, n.compiled.Manager, result.Updates); err != nil {
			return fmt.Errorf("failed to apply state updates: %w", err)
		}

		// Extract messages from updates and yield directly
		// Note: Uses "__messages__" key name (defined in agent.MessagesKey)
		if messagesAny, ok := result.Updates["__messages__"]; ok {
			if messages, ok := messagesAny.([]message.Message); ok && len(messages) > 0 {
				// Yield each message directly
				for _, msg := range messages {
					if !n.yield(msg, nil) {
						return nil
					}
				}
			}
		}
	}

	// Prepare message data for distributed state if enabled
	var messageData message.Message
	if n.enableDistributedState {
		// Extract messages from updates for distributed state
		// Note: Uses "__messages__" key name (defined in agent.MessagesKey)
		var messages []message.Message
		if messagesAny, ok := result.Updates["__messages__"]; ok {
			if msgs, ok := messagesAny.([]message.Message); ok {
				messages = msgs
			}
		}

		stateMsg := NewStateMessage(messages, result.Updates)
		messageData = stateMsg.ToMessage()
	}

	// Send messages to next nodes via pregel runtime
	// Check for conditional edges first
	if conditionals, ok := n.compiled.Topology.ConditionalByFrom[n.nodeName]; ok {
		// Note: Conditional edges evaluate against current superstep's state (before barrier)
		// This is semantically correct for routing decisions based on current node's output
		condView, err := n.compiled.Manager.CreateReadView(ctx)
		if err != nil {
			return fmt.Errorf("failed to create read view for conditionals: %w", err)
		}
		for _, cond := range conditionals {
			targets := cond.Condition(ctx, condView)
			for _, target := range targets {
				if target != n.compiled.EndNode {
					vertex.Send(pregel.Message[message.Message]{
						From: n.nodeName,
						To:   target,
						Data: messageData,
					})
				}
			}
		}
	} else {
		// Use regular outgoing edges
		for _, target := range vertex.State.Topology.Outgoing[n.nodeName] {
			if target != n.compiled.EndNode {
				vertex.Send(pregel.Message[message.Message]{
					From: n.nodeName,
					To:   target,
					Data: messageData,
				})
			}
		}
	}

	return nil
}
