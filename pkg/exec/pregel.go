package exec

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"runtime"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Default key name for messages (same as agent.MessagesKey.Name()).
// Defined here to avoid import cycle with agent package.
const defaultMessagesKeyName = "__messages__"

// isSlice checks if a value is a slice type using reflection.
func isSlice(value any) bool {
	if value == nil {
		return false
	}
	return reflect.TypeOf(value).Kind() == reflect.Slice
}

// reflectSliceLen returns the length of a slice using reflection.
func reflectSliceLen(value any) int {
	return reflect.ValueOf(value).Len()
}

// reflectSliceIndex returns the element at index i of a slice using reflection.
func reflectSliceIndex(value any, i int) any {
	return reflect.ValueOf(value).Index(i).Interface()
}

// Pregel is a Bulk-Synchronous Parallel (BSP) executor that uses the pregel runtime.
type Pregel[I, O any] struct {
	maxWorkers             int
	maxIters               int
	messageBus             pregel.MessageBus[state.Updates]
	aggregators            map[string]pregel.Aggregator
	enableDistributedState bool            // Enable state synchronization via message bus
	metrics                *RuntimeMetrics // Track execution metadata for checkpoints

	// Generic executor configuration
	inputToState  func(I) state.Updates // Convert input to initial state
	outputKey     string                // Which state key to yield as output
	outputAdapter func(any) O           // Convert state value to output type
}

// PregelOption configures a Pregel executor.
type PregelOption[I, O any] func(*Pregel[I, O])

// WithMaxWorkers sets the maximum number of parallel workers.
func WithMaxWorkers[I, O any](n int) PregelOption[I, O] {
	return func(p *Pregel[I, O]) {
		if n > 0 {
			p.maxWorkers = n
		}
	}
}

// WithMaxIterations sets the maximum number of supersteps.
func WithMaxIterations[I, O any](n int) PregelOption[I, O] {
	return func(p *Pregel[I, O]) {
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
func WithMessageBus[I, O any](bus pregel.MessageBus[state.Updates]) PregelOption[I, O] {
	return func(p *Pregel[I, O]) {
		p.messageBus = bus
		p.enableDistributedState = true // Auto-enable when message bus is provided
	}
}

// WithAggregators configures global aggregators.
func WithAggregators[I, O any](aggs map[string]pregel.Aggregator) PregelOption[I, O] {
	return func(p *Pregel[I, O]) {
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
func WithDistributedState[I, O any](enable ...bool) PregelOption[I, O] {
	enabled := true
	if len(enable) > 0 {
		enabled = enable[0]
	}
	return func(p *Pregel[I, O]) {
		p.enableDistributedState = enabled
	}
}

// NewPregelExecutor creates the default message-based Pregel executor.
// This is the standard executor for agent systems, chat workflows.
// Input: []message.Message, Output: message.Message (individual messages)
//
// Note: The executor automatically unfolds message arrays. When a node adds multiple
// messages (e.g., parallel tool calls), each message is yielded separately to the stream.
func NewPregelExecutor(opts ...PregelOption[[]message.Message, message.Message]) *Pregel[[]message.Message, message.Message] {
	return NewGenericPregelExecutor[[]message.Message, message.Message](
		// Input: Convert []message.Message to state using default messages key
		func(input []message.Message) state.Updates {
			if len(input) == 0 {
				return nil
			}
			return state.Updates{defaultMessagesKeyName: input}
		},
		// Output: Watch default messages key
		defaultMessagesKeyName,
		// Output: Identity adapter - messages are unfolded by the executor
		func(value any) message.Message {
			if msg, ok := value.(message.Message); ok {
				return msg
			}
			return nil
		},
		opts...,
	)
}

// NewStatePregelExecutor creates a state-only Pregel executor.
// This is for pure state transformation workflows, data pipelines, ETL.
// Input: state.Updates, Output: state.Updates (all state updates)
func NewStatePregelExecutor(opts ...PregelOption[state.Updates, state.Updates]) *Pregel[state.Updates, state.Updates] {
	return NewGenericPregelExecutor[state.Updates, state.Updates](
		// Input: Use provided state.Updates directly as initial state
		func(input state.Updates) state.Updates {
			return input
		},
		// Output: Special marker to yield all updates (not just one key)
		"*", // Wildcard means "yield entire state.Updates"
		// Output: Return updates as-is
		func(value any) state.Updates {
			if updates, ok := value.(state.Updates); ok {
				return updates
			}
			return nil
		},
		opts...,
	)
}

// NewKeyPregelExecutor creates a key-based Pregel executor.
// This is for domain-specific workflows with typed input/output.
// Input: type I stored in inputKey, Output: type O from outputKey
func NewKeyPregelExecutor[I, O any](
	inputKey *state.Key[I],
	outputKey *state.Key[O],
	opts ...PregelOption[I, O],
) *Pregel[I, O] {
	return NewGenericPregelExecutor[I, O](
		// Input: Store input in specified key
		func(input I) state.Updates {
			return state.Updates{inputKey.Name(): input}
		},
		// Output: Watch specified key
		outputKey.Name(),
		// Output: Type-safe extraction
		func(value any) O {
			if typed, ok := value.(O); ok {
				return typed
			}
			var zero O
			return zero
		},
		opts...,
	)
}

// NewGenericPregelExecutor creates a fully customizable Pregel executor.
// This is for advanced use cases with custom input/output transformations.
//
// Parameters:
//   - inputToState: Converts input I to initial state updates
//   - outputKey: Which state key to watch and yield (use "*" for all updates)
//   - outputAdapter: Converts state value to output type O
//   - opts: Additional configuration options
func NewGenericPregelExecutor[I, O any](
	inputToState func(I) state.Updates,
	outputKey string,
	outputAdapter func(any) O,
	opts ...PregelOption[I, O],
) *Pregel[I, O] {
	p := &Pregel[I, O]{
		maxWorkers:    runtime.NumCPU(),
		maxIters:      1000,
		metrics:       NewRuntimeMetrics(),
		inputToState:  inputToState,
		outputKey:     outputKey,
		outputAdapter: outputAdapter,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run executes the compiled graph using the pregel BSP runtime.
//
//nolint:gocyclo // Complex graph orchestration logic cannot be easily simplified
func (p *Pregel[I, O]) Run(
	ctx context.Context,
	compiled *compile.CompiledGraph,
	input I,
	opts ...graph.RunOption) iter.Seq2[O, error] {
	return func(yield func(O, error) bool) {
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
			var zero O
			yield(zero, err)
			return
		}

		// Start checkpoint worker
		worker := p.startCheckpointWorker(runCtx, runOpts)
		defer p.stopCheckpointWorker(worker)

		// Convert input to initial state using adapter
		var inputValue any = input
		if inputValue != nil {
			initialState := p.inputToState(input)
			if len(initialState) > 0 {
				if err := state.ApplyUpdates(runCtx, compiled.Manager, initialState); err != nil {
					var zero O
					yield(zero, fmt.Errorf("failed to apply initial state: %w", err))
					return
				}
			}
		}

		// Create result channel to serialize all yields from a single goroutine
		resultChan := make(chan struct {
			output O
			err    error
		}, 100)

		// Single goroutine that calls yield - ensures thread safety
		yieldDone := make(chan struct{})
		go func() {
			defer close(yieldDone)
			for item := range resultChan {
				if !yield(item.output, item.err) {
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
		safeYield := func(output O, err error) bool {
			select {
			case resultChan <- struct {
				output O
				err    error
			}{output, err}:
				return true
			case <-runCtx.Done():
				return false
			}
		}

		// Create adapter to make compiled graph work with pregel runtime
		adapter := &pregelGraphAdapter[I, O]{
			compiled:               compiled,
			runID:                  runID,
			yield:                  safeYield,
			enableDistributedState: p.enableDistributedState,
			executor:               p,
		}

		// Configure pregel runtime options
		runtimeOpts := []pregel.RuntimeOption[*compile.CompiledGraph, state.Updates]{
			pregel.WithMaxWorkers[*compile.CompiledGraph, state.Updates](p.maxWorkers),
			pregel.WithMaxIterations[*compile.CompiledGraph, state.Updates](p.maxIters),
		}

		if p.messageBus != nil {
			runtimeOpts = append(runtimeOpts, pregel.WithMessageBus[*compile.CompiledGraph, state.Updates](p.messageBus))
		}

		if len(p.aggregators) > 0 {
			runtimeOpts = append(runtimeOpts, pregel.WithAggregators[*compile.CompiledGraph, state.Updates](p.aggregators))
		}

		// Add superstep completion callback for checkpointing
		if runOpts.Checkpointer != nil && runOpts.RunID != "" {
			runtimeOpts = append(runtimeOpts, pregel.WithOnSuperstepComplete[*compile.CompiledGraph, state.Updates](
				func(ctx context.Context, superstep int64) {
					p.saveCheckpoint(ctx, compiled, runOpts, superstep, worker)
				},
			))
		}

		// Create and run the pregel runtime
		rt, err := pregel.NewRuntime[*compile.CompiledGraph, state.Updates](adapter, runtimeOpts...)
		if err != nil {
			var zero O
			safeYield(zero, err)
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
				var zero O
				safeYield(zero, err)
				break
			}
		}

		close(resultChan)
		<-yieldDone // Wait for yield goroutine to finish
	}
}

// pregelGraphAdapter adapts a CompiledGraph to the pregel.Graph interface.
type pregelGraphAdapter[I, O any] struct {
	compiled               *compile.CompiledGraph
	runID                  string
	yield                  func(O, error) bool
	enableDistributedState bool
	executor               *Pregel[I, O] // Reference to executor for metrics tracking
}

// RootNodes returns nodes with no incoming edges.
func (a *pregelGraphAdapter[I, O]) RootNodes() []string {
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

// Outgoing returns outgoing edges for a node.
func (a *pregelGraphAdapter[I, O]) Outgoing(nodeName string) []string {
	return a.compiled.Topology.Outgoing[nodeName]
}

// NodeByName returns a pregel node adapter.
func (a *pregelGraphAdapter[I, O]) NodeByName(name string) pregel.Node[*compile.CompiledGraph, state.Updates] {
	return &pregelNodeAdapter[I, O]{
		nodeName:               name,
		compiled:               a.compiled,
		runID:                  a.runID,
		yield:                  a.yield,
		enableDistributedState: a.enableDistributedState,
		executor:               a.executor,
	}
}

// State returns the compiled graph (used as global state).
func (a *pregelGraphAdapter[I, O]) State() *compile.CompiledGraph {
	return a.compiled
}

// pregelNodeAdapter adapts a graph.Node to pregel.Node interface.
type pregelNodeAdapter[I, O any] struct {
	nodeName               string
	compiled               *compile.CompiledGraph
	runID                  string
	yield                  func(O, error) bool
	enableDistributedState bool
	executor               *Pregel[I, O] // Reference to executor for metrics tracking
}

// Name returns the node name.
func (n *pregelNodeAdapter[I, O]) Name() string {
	return n.nodeName
}

// Run executes the node.
//
//nolint:gocyclo,nestif // BSP node execution requires complex state management
func (n *pregelNodeAdapter[I, O]) Run(
	ctx context.Context,
	vertex pregel.VertexContext[*compile.CompiledGraph, state.Updates],
	incoming []pregel.Message[state.Updates],
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

	// Apply incoming state updates from distributed nodes (BSP synchronization)
	// In distributed execution, predecessor nodes send their state.Updates via the message bus
	// This ensures BSP consistency: each node sees all updates from previous superstep
	if n.enableDistributedState && len(incoming) > 0 {
		for _, msg := range incoming {
			if len(msg.Data) == 0 {
				continue // Routing signal only, no state
			}

			// msg.Data is already state.Updates - apply directly
			if err := state.ApplyUpdates(ctx, n.compiled.Manager, msg.Data); err != nil {
				return fmt.Errorf("failed to apply distributed state updates: %w", err)
			}
		}
	}

	// Create a stream writer that yields intermediate results
	streamWriter := func(intermediateResult *graph.NodeResult) {
		if intermediateResult == nil {
			return
		}

		// Extract output from updates based on configured key
		if n.executor.outputKey == "*" {
			// Yield entire state.Updates (for state-only executor)
			output := n.executor.outputAdapter(intermediateResult.Updates)
			n.yield(output, nil)
		} else if value, ok := intermediateResult.Updates[n.executor.outputKey]; ok {
			// Yield specific key value
			output := n.executor.outputAdapter(value)
			n.yield(output, nil)
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

		// Extract output from updates based on configured key and yield
		if n.executor.outputKey == "*" {
			// Yield entire state.Updates (for state-only executor)
			output := n.executor.outputAdapter(result.Updates)
			if !n.yield(output, nil) {
				return nil
			}
		} else if value, ok := result.Updates[n.executor.outputKey]; ok {
			// Special handling for slices: unfold and yield each element individually
			// This allows nodes to return multiple outputs (e.g., parallel tool calls)
			// and have each one appear in the output stream
			if isSlice(value) {
				sliceLen := reflectSliceLen(value)
				for i := range sliceLen {
					elem := reflectSliceIndex(value, i)
					output := n.executor.outputAdapter(elem)
					if !n.yield(output, nil) {
						return nil
					}
				}
			} else {
				// For non-slice values, apply adapter and yield once
				output := n.executor.outputAdapter(value)
				if !n.yield(output, nil) {
					return nil
				}
			}
		}
	}

	// Send routing signals (and optionally state) to next nodes via pregel runtime
	// For distributed execution, state updates are sent directly as state.Updates
	var stateData state.Updates
	if n.enableDistributedState && len(result.Updates) > 0 {
		// Send state.Updates directly for distributed synchronization
		// Remote nodes will receive and apply these updates before execution
		stateData = result.Updates
	}

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
					vertex.Send(pregel.Message[state.Updates]{
						From: n.nodeName,
						To:   target,
						Data: stateData,
					})
				}
			}
		}
	} else {
		// Use regular outgoing edges
		for _, target := range vertex.State.Topology.Outgoing[n.nodeName] {
			if target != n.compiled.EndNode {
				vertex.Send(pregel.Message[state.Updates]{
					From: n.nodeName,
					To:   target,
					Data: stateData,
				})
			}
		}
	}

	return nil
}
