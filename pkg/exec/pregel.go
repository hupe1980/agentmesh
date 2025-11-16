package exec

import (
	"context"
	"fmt"
	"iter"
	"runtime"
	"time"

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
	enableDistributedState bool // Enable state synchronization via message bus
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
	initialMessages []message.Message,
	opts ...graph.RunOption) iter.Seq2[state.ExecutionResult, error] {
	return func(yield func(state.ExecutionResult, error) bool) {
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
			yield(state.ExecutionResult{}, err)
			return
		}

		// Start checkpoint worker
		worker := p.startCheckpointWorker(runCtx, runOpts)
		defer p.stopCheckpointWorker(worker)

		// Add initial messages to state
		if len(initialMessages) > 0 {
			events := make([]state.ExecutionResult, len(initialMessages))
			for i, msg := range initialMessages {
				events[i] = state.ExecutionResult{
					Message:   msg,
					ID:        uuid.New().String(),
					GraphID:   runID,
					Node:      compiled.StartNode,
					Timestamp: time.Now(),
				}
			}
			compiled.StateManager.AddMessages(events)
		}

		// Create result channel to serialize all yields from a single goroutine
		resultChan := make(chan struct {
			result state.ExecutionResult
			err    error
		}, 100)

		// Single goroutine that calls yield - ensures thread safety
		yieldDone := make(chan struct{})
		go func() {
			defer close(yieldDone)
			for item := range resultChan {
				if !yield(item.result, item.err) {
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
		safeYield := func(result state.ExecutionResult, err error) bool {
			select {
			case resultChan <- struct {
				result state.ExecutionResult
				err    error
			}{result, err}:
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

		// Add checkpoint callback
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
			safeYield(state.ExecutionResult{}, err)
			close(resultChan)
			<-yieldDone
			return
		}

		// Execute runtime and forward events to result channel
		for evt, err := range rt.Run(runCtx) {
			if err != nil || evt.Error != nil {
				// Forward error event
				safeYield(state.ExecutionResult{
					ID:        uuid.New().String(),
					GraphID:   runID,
					Node:      evt.Node,
					Timestamp: time.Now(),
					Err:       err,
				}, err)
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
	yield                  func(state.ExecutionResult, error) bool
	enableDistributedState bool
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
	yield                  func(state.ExecutionResult, error) bool
	enableDistributedState bool
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

	// If distributed state is enabled, apply incoming state updates first
	if n.enableDistributedState {
		for _, msg := range incoming {
			if stateMsg := FromMessage(msg.Data); stateMsg != nil {
				// Apply state updates from remote node
				for key, value := range stateMsg.Updates {
					// Handle JSON numeric type conversion (float64 -> int)
					// When JSON deserializes numbers, they become float64
					// Convert back to int if the original value was an int
					if existing := n.compiled.StateManager.Get(key); existing != nil {
						switch existing.(type) {
						case int:
							if f, ok := value.(float64); ok {
								value = int(f)
							}
						case int64:
							if f, ok := value.(float64); ok {
								value = int64(f)
							}
						case int32:
							if f, ok := value.(float64); ok {
								value = int32(f)
							}
						}
					}
					if err := n.compiled.StateManager.Set(key, value); err != nil {
						return fmt.Errorf("failed to apply distributed state update for key %q: %w", key, err)
					}
				}

				// Add messages to local state
				if len(stateMsg.Messages) > 0 {
					n.compiled.StateManager.AddMessages(stateMsg.Messages)
				}
			}
		}
	}

	// Create a stream writer that yields intermediate results
	streamWriter := func(intermediateResult *graph.NodeResult) {
		if intermediateResult == nil {
			return
		}

		// Create execution events for intermediate messages
		if len(intermediateResult.Messages) > 0 {
			events := make([]state.ExecutionResult, len(intermediateResult.Messages))
			for i, msg := range intermediateResult.Messages {
				events[i] = state.ExecutionResult{
					Message:   msg,
					ID:        uuid.New().String(),
					GraphID:   n.runID,
					Node:      n.nodeName,
					Timestamp: time.Now(),
					Partial:   true, // Mark as intermediate/partial result
				}
			}

			// Yield each intermediate event
			for _, event := range events {
				n.yield(event, nil)
			}
		}

		// If there are state updates in the intermediate result,
		// create a synthetic event to carry them
		if len(intermediateResult.Updates) > 0 {
			n.yield(state.ExecutionResult{
				ID:        uuid.New().String(),
				GraphID:   n.runID,
				Node:      n.nodeName,
				Timestamp: time.Now(),
				Partial:   true,
				Updates:   intermediateResult.Updates,
			}, nil)
		}
	}

	// Attach stream writer to context
	ctxWithStream := graph.WithStreamWriter(ctx, streamWriter)

	// Execute the node with streaming support
	result, err := node.Run(ctxWithStream, n.compiled.StateManager)
	if err != nil {
		return err
	}

	if result == nil {
		return nil
	}

	// Update state locally
	for key, value := range result.Updates {
		if err := n.compiled.StateManager.Set(key, value); err != nil {
			return fmt.Errorf("failed to update state for key %q: %w", key, err)
		}
	}

	// Add messages to state and yield events
	if len(result.Messages) > 0 {
		events := make([]state.ExecutionResult, len(result.Messages))
		for i, msg := range result.Messages {
			events[i] = state.ExecutionResult{
				Message:   msg,
				ID:        uuid.New().String(),
				GraphID:   n.runID,
				Node:      n.nodeName,
				Timestamp: time.Now(),
			}
		}
		n.compiled.StateManager.AddMessages(events)

		// Yield each event
		for _, event := range events {
			if !n.yield(event, nil) {
				return nil
			}
		}
	}

	// Prepare message data for distributed state if enabled
	var messageData message.Message
	if n.enableDistributedState {
		// Create state message with updates
		events := make([]state.ExecutionResult, len(result.Messages))
		for i, msg := range result.Messages {
			events[i] = state.ExecutionResult{
				Message:   msg,
				ID:        uuid.New().String(),
				GraphID:   n.runID,
				Node:      n.nodeName,
				Timestamp: time.Now(),
			}
		}

		stateMsg := NewStateMessage(events, result.Updates)
		messageData = stateMsg.ToMessage()
	}

	// Send messages to next nodes via pregel runtime
	// Check for conditional edges first
	if conditionals, ok := n.compiled.Topology.ConditionalByFrom[n.nodeName]; ok {
		for _, cond := range conditionals {
			targets := cond.Condition(ctx, n.compiled.StateManager)
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
