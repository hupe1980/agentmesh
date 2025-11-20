package graph

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/state"
)

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

// PregelExecutor is a Bulk-Synchronous Parallel (BSP) executor using the pregel runtime.
// This is the default executor for agent workflows and chat systems.
//
// # BSP Semantics (Hybrid Model)
//
// AgentMesh implements a hybrid BSP model that balances theoretical purity with practical
// agent workflow requirements:
//
// BSP-Compliant Features:
//   - Inter-node isolation: Nodes read from shared superstep snapshot
//   - Superstep barriers: All nodes finish before next superstep begins
//   - Distributed state: Updates propagated via messages, delivered next superstep
//   - Aggregators: Thread-safe accumulation across nodes
//
// Non-BSP Features (Deliberate Trade-offs):
//   - Conditional edge routing: Evaluates using FRESH state (sees node's own updates)
//   - Immediate local updates: Applied per-node (not buffered until barrier)
//
// Why Violate Pure BSP?
//
// Agent patterns (ReAct) require conditional routing to see node outputs:
//
//	Model → [AIMessage with tool_calls] → Route based on tool_calls → Tool Executor
//
// Pure BSP would use superstep snapshot for routing, which can't see the AIMessage.
// This breaks agent workflows. The hybrid model maintains inter-node isolation while
// allowing intra-node visibility for routing decisions.
//
// See _prompts/BSP_SEMANTICS.md for detailed analysis.
type PregelExecutor[I, O any] struct {
	maxWorkers             int
	maxIters               int
	messageBus             pregel.MessageBus[state.Updates]
	aggregators            map[string]pregel.Aggregator
	enableDistributedState bool
	metrics                *RuntimeMetrics

	// Generic executor configuration
	inputToState  func(I) state.Updates // Convert input to initial state
	outputKey     string                // Which state key to yield as output
	outputAdapter func(any) O           // Convert state value to output type
}

// PregelOption configures a Pregel executor.
type PregelOption[I, O any] func(*PregelExecutor[I, O])

// WithMaxWorkers sets the maximum number of parallel workers.
func WithMaxWorkers[I, O any](n int) PregelOption[I, O] {
	return func(p *PregelExecutor[I, O]) {
		if n > 0 {
			p.maxWorkers = n
		}
	}
}

// WithPregelMaxIterations sets the maximum number of supersteps for the Pregel executor.
func WithPregelMaxIterations[I, O any](n int) PregelOption[I, O] {
	return func(p *PregelExecutor[I, O]) {
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
	return func(p *PregelExecutor[I, O]) {
		p.messageBus = bus
		p.enableDistributedState = true // Auto-enable when message bus is provided
	}
}

// WithAggregators configures global aggregators.
func WithAggregators[I, O any](aggs map[string]pregel.Aggregator) PregelOption[I, O] {
	return func(p *PregelExecutor[I, O]) {
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
	return func(p *PregelExecutor[I, O]) {
		p.enableDistributedState = enabled
	}
}

// NewMessagePregelExecutor creates the default message-based Pregel executor.
// This is the standard executor for agent systems, chat workflows.
// Input: []message.Message, Output: message.Message (individual messages)
//
// Note: The executor automatically unfolds message arrays. When a node adds multiple
// messages (e.g., parallel tool calls), each message is yielded separately to the stream.
func NewMessagePregelExecutor(opts ...PregelOption[[]message.Message, message.Message]) *PregelExecutor[[]message.Message, message.Message] {
	return NewPregelExecutor(
		// Input: Convert []message.Message to state using standard messages key
		func(input []message.Message) state.Updates {
			if len(input) == 0 {
				return nil
			}
			return state.Updates{MessagesKeyName: state.SliceOf[message.Message](input)}
		},
		// Output: Watch standard messages key
		MessagesKeyName,
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
func NewStatePregelExecutor(opts ...PregelOption[state.Updates, state.Updates]) *PregelExecutor[state.Updates, state.Updates] {
	return NewPregelExecutor(
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
) *PregelExecutor[I, O] {
	return NewPregelExecutor(
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

// NewPregelExecutor creates a fully customizable Pregel executor.
// This is for advanced use cases with custom input/output transformations.
//
// Parameters:
//   - inputToState: Converts input I to initial state updates
//   - outputKey: Which state key to watch and yield (use "*" for all updates)
//   - outputAdapter: Converts state value to output type O
//   - opts: Additional configuration options
func NewPregelExecutor[I, O any](
	inputToState func(I) state.Updates,
	outputKey string,
	outputAdapter func(any) O,
	opts ...PregelOption[I, O],
) *PregelExecutor[I, O] {
	p := &PregelExecutor[I, O]{
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
func (p *PregelExecutor[I, O]) Run(
	ctx context.Context,
	compiled *Compiled[I, O],
	input I,
	opts ...RunOption) iter.Seq2[O, error] {
	return func(yield func(O, error) bool) {
		// Extract run options
		runOpts := ApplyOptions(opts...)

		// Create a cancellable context so we can stop the runtime when consumer exits
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Inject resume values into context if provided
		if runOpts.ResumeValue != nil {
			runCtx = withResumeValueContext(runCtx, runOpts.ResumeValue)
		}

		runID := runOpts.RunID
		if runID == "" {
			runID = uuid.New().String()
		}

		// Restore from checkpoint if configured
		if err := p.restoreCheckpoint(runCtx, compiled, runOpts); err != nil {
			var zero O
			yield(zero, err)
			return
		} // Start checkpoint worker
		worker := p.startCheckpointWorker(runCtx, runOpts)
		defer p.stopCheckpointWorker(worker)

		// Convert input to initial state using adapter
		var inputValue any = input
		if inputValue != nil {
			initialState := p.inputToState(input)
			if len(initialState) > 0 {
				if err := compiled.manager.ApplyUpdates(runCtx, initialState); err != nil {
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
			checkpointer:           runOpts.Checkpointer, // For interrupt checkpoints
		} // Configure pregel runtime options
		runtimeOpts := []pregel.RuntimeOption[*Compiled[I, O], state.Updates]{
			pregel.WithMaxWorkers[*Compiled[I, O], state.Updates](p.maxWorkers),
			pregel.WithMaxIterations[*Compiled[I, O], state.Updates](p.maxIters),
		}

		if p.messageBus != nil {
			runtimeOpts = append(runtimeOpts, pregel.WithMessageBus[*Compiled[I, O], state.Updates](p.messageBus))
		}

		if len(p.aggregators) > 0 {
			runtimeOpts = append(runtimeOpts, pregel.WithAggregators[*Compiled[I, O], state.Updates](p.aggregators))
		}

		// Add BSP barrier callbacks for proper state snapshot management
		runtimeOpts = append(runtimeOpts, pregel.WithOnSuperstepStart[*Compiled[I, O], state.Updates](
			func(ctx context.Context, superstep int64) error {
				// Create snapshot at start of superstep (BSP read barrier)
				return adapter.prepareSuperstep(ctx)
			},
		))

		runtimeOpts = append(runtimeOpts, pregel.WithOnSuperstepComplete[*Compiled[I, O], state.Updates](
			func(ctx context.Context, superstep int64) error {
				// Save checkpoint after superstep completes
				// Updates are already applied immediately per node
				if runOpts.Checkpointer != nil && runOpts.RunID != "" {
					p.saveCheckpoint(ctx, compiled, runOpts, superstep, worker)
				}
				return nil
			},
		))

		// Create and run the pregel runtime
		rt, err := pregel.NewRuntime[*Compiled[I, O], state.Updates](adapter, runtimeOpts...)
		if err != nil {
			var zero O
			safeYield(zero, err)
			close(resultChan)
			<-yieldDone
			return
		}

		// Execute runtime and forward events to result channel
		for _, err := range rt.Run(runCtx) {
			// Update metrics with current superstep
			if p.metrics != nil {
				p.metrics.SetSuperstep(rt.CurrentSuperstep())
			}

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

// pregelGraphAdapter adapts a Compiled[I,O] to the pregel.Graph interface.
type pregelGraphAdapter[I, O any] struct {
	compiled               *Compiled[I, O]
	runID                  string
	yield                  func(O, error) bool
	enableDistributedState bool
	executor               *PregelExecutor[I, O]
	checkpointer           checkpoint.Checkpointer // For interrupt checkpoints

	// BSP barrier support: one ReadView snapshot per superstep
	// Nodes read from this shared snapshot for BSP-correct state access
	currentSuperstepView *state.ReadView
	mu                   sync.RWMutex
}

// RootNodes returns nodes with no incoming edges.
func (a *pregelGraphAdapter[I, O]) RootNodes() []string {
	// If resuming from a checkpoint, return the resuming nodes as roots
	if a.executor != nil && a.executor.metrics != nil {
		snapshot := a.executor.metrics.Snapshot()
		if len(snapshot.ResumingNodes) > 0 {
			// When resuming, start execution from the paused nodes
			return snapshot.ResumingNodes
		}
	}

	// Normal case: return actual root nodes (nodes with no incoming edges)
	var roots []string
	for _, nodeName := range a.compiled.topology.nodeNames {
		if nodeName == StartNode || nodeName == EndNode {
			continue
		}
		if a.compiled.topology.incoming[nodeName] == 0 {
			roots = append(roots, nodeName)
		}
	}
	return roots
}

// Outgoing returns outgoing edges for a node.
func (a *pregelGraphAdapter[I, O]) Outgoing(nodeName string) []string {
	return a.compiled.topology.outgoing[nodeName]
}

// NodeByName returns a pregel node adapter.
func (a *pregelGraphAdapter[I, O]) NodeByName(name string) pregel.Node[*Compiled[I, O], state.Updates] {
	return &pregelNodeAdapter[I, O]{
		nodeName:               name,
		compiled:               a.compiled,
		runID:                  a.runID,
		yield:                  a.yield,
		enableDistributedState: a.enableDistributedState,
		executor:               a.executor,
		graphAdapter:           a,              // Pass adapter for BSP access
		checkpointer:           a.checkpointer, // For interrupt checkpoints
	}
}

// State returns the compiled graph (used as global state).
func (a *pregelGraphAdapter[I, O]) State() *Compiled[I, O] {
	return a.compiled
}

// prepareSuperstep creates a snapshot for BSP-correct state reads.
// Called once at the start of each superstep.
// All nodes in the superstep will read from this shared snapshot.
func (a *pregelGraphAdapter[I, O]) prepareSuperstep(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create read view for this superstep (all nodes will share this snapshot)
	view, err := a.compiled.manager.CreateReadView(ctx)
	if err != nil {
		return fmt.Errorf("failed to create superstep snapshot: %w", err)
	}

	a.currentSuperstepView = view
	return nil
}

// getSuperstepView returns the read-only view for the current superstep.
func (a *pregelGraphAdapter[I, O]) getSuperstepView() *state.ReadView {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentSuperstepView
}

// pregelNodeAdapter adapts a graph.Node to pregel.Node interface.
type pregelNodeAdapter[I, O any] struct {
	nodeName               string
	compiled               *Compiled[I, O]
	runID                  string
	yield                  func(O, error) bool
	enableDistributedState bool
	executor               *PregelExecutor[I, O]
	graphAdapter           *pregelGraphAdapter[I, O] // For BSP barrier access
	checkpointer           checkpoint.Checkpointer   // For saving interrupt checkpoints
}

// Name returns the node name.
func (n *pregelNodeAdapter[I, O]) Name() string {
	return n.nodeName
}

// executeWithPolicies executes a node with retry and cache policies applied.
func (n *pregelNodeAdapter[I, O]) executeWithPolicies(
	ctx context.Context,
	node Node,
	view *state.ReadView,
) (state.Updates, error) {
	config := n.compiled.graph.NodeConfigs[n.nodeName]
	if config == nil {
		config = defaultNodeConfig()
	}

	// Try cache if enabled
	if n.shouldUseCache(config) {
		return n.executeWithCache(ctx, node, view, config)
	}

	// No caching - execute with retry
	return n.executeWithRetry(ctx, node, view, config.RetryPolicy)
}

// shouldUseCache checks if caching is enabled and configured.
func (n *pregelNodeAdapter[I, O]) shouldUseCache(config *NodeConfig) bool {
	return config.CachePolicy != nil &&
		config.CachePolicy.Enabled &&
		config.CachePolicy.KeyFunc != nil &&
		config.CachePolicy.Store != nil
}

// executeWithCache attempts to use cache, falling back to execution if needed.
func (n *pregelNodeAdapter[I, O]) executeWithCache(
	ctx context.Context,
	node Node,
	view *state.ReadView,
	config *NodeConfig,
) (state.Updates, error) {
	store := config.CachePolicy.Store
	cacheKey := n.generateCacheKey(ctx, view, config)

	// Try cache lookup
	if cacheKey != "" {
		if updates, ok := n.getCachedResult(ctx, store, cacheKey, config); ok {
			return updates, nil
		}
	}

	// Cache miss - execute and store
	updates, err := n.executeWithRetry(ctx, node, view, config.RetryPolicy)
	if err == nil && cacheKey != "" && updates != nil {
		n.cacheResult(ctx, store, cacheKey, updates, config)
	}

	return updates, err
}

// generateCacheKey creates a cache key from the current state.
func (n *pregelNodeAdapter[I, O]) generateCacheKey(
	ctx context.Context,
	view *state.ReadView,
	config *NodeConfig,
) string {
	snapshotData := make(map[string]any)
	for _, key := range view.Keys() {
		if view.Has(key) {
			snapshotData[key] = true // Placeholder - KeyFunc should use GetFromView
		}
	}
	return config.CachePolicy.KeyFunc(ctx, snapshotData)
}

// getCachedResult attempts to retrieve a valid cached result.
func (n *pregelNodeAdapter[I, O]) getCachedResult(
	ctx context.Context,
	store CacheStore,
	cacheKey string,
	config *NodeConfig,
) (state.Updates, bool) {
	entry, err := store.Get(ctx, cacheKey)
	if err != nil || entry.Updates == nil {
		return nil, false
	}

	// Check TTL
	if config.CachePolicy.TTL > 0 && time.Since(entry.Timestamp) >= config.CachePolicy.TTL {
		return nil, false
	}

	return entry.Updates, true
}

// cacheResult stores execution results in the cache.
func (n *pregelNodeAdapter[I, O]) cacheResult(
	ctx context.Context,
	store CacheStore,
	cacheKey string,
	updates state.Updates,
	config *NodeConfig,
) {
	_ = store.Set(ctx, cacheKey, CacheEntry{
		Updates:   updates,
		Timestamp: time.Now(),
	}, config.CachePolicy.TTL)
}

// executeWithRetry executes a node with retry policy applied.
func (n *pregelNodeAdapter[I, O]) executeWithRetry(
	ctx context.Context,
	node Node,
	view *state.ReadView,
	policy *RetryPolicy,
) (state.Updates, error) {
	if policy == nil || policy.MaxAttempts <= 1 {
		// No retry policy or only 1 attempt - execute directly
		return node.Execute(ctx, view)
	}

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		// Execute node
		updates, err := node.Execute(ctx, view)
		if err == nil {
			return updates, nil
		}

		lastErr = err

		// Check if error is retryable
		if policy.Retryable != nil && !policy.Retryable(err) {
			// Not retryable - fail immediately
			return nil, err
		}

		// Don't sleep after the last attempt
		if attempt < policy.MaxAttempts {
			// Apply backoff
			if policy.Backoff != nil {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(policy.Backoff(attempt)):
					// Continue to next attempt
				}
			}
		}
	}

	return nil, fmt.Errorf("max retry attempts (%d) exceeded: %w", policy.MaxAttempts, lastErr)
}

// shouldInterruptBefore checks if this node is in the interrupt-before list.
func (n *pregelNodeAdapter[I, O]) shouldInterruptBefore() bool {
	return slices.Contains(n.compiled.graph.InterruptBefore, n.nodeName)
}

// shouldInterruptAfter checks if this node is in the interrupt-after list.
func (n *pregelNodeAdapter[I, O]) shouldInterruptAfter() bool {
	return slices.Contains(n.compiled.graph.InterruptAfter, n.nodeName)
}

// createInterruptCheckpoint creates a checkpoint with pending writes and pauses execution.
func (n *pregelNodeAdapter[I, O]) createInterruptCheckpoint(
	ctx context.Context,
	vertex pregel.VertexContext[*Compiled[I, O], state.Updates],
	updates state.Updates,
	isBefore bool,
) {
	if n.executor == nil {
		return // No executor, can't create checkpoint
	}

	logger := logging.FromContext(ctx)
	stage := "before"
	if !isBefore {
		stage = "after"
	}

	logger.Info("interrupt detected, creating checkpoint",
		"node", n.nodeName,
		"stage", stage)

	// Get current state snapshot
	vsnap, err := n.compiled.manager.Snapshot(ctx, map[string]string{
		"run_id":    n.runID,
		"node":      n.nodeName,
		"interrupt": stage,
	})
	if err != nil {
		logger.Error("failed to create state snapshot for interrupt",
			"node", n.nodeName,
			"error", err)
		return
	}

	// Create pending writes from updates (if after execution)
	var pendingWrites []checkpoint.PendingWrite
	if !isBefore && len(updates) > 0 {
		for channel, value := range updates {
			pendingWrites = append(pendingWrites, checkpoint.PendingWrite{
				NodeName:  n.nodeName,
				Channel:   channel,
				Value:     value,
				Timestamp: vsnap.Timestamp,
			})
		}
	}

	// Capture execution metadata
	var completedNodes, pausedNodes []string
	if n.executor.metrics != nil {
		snapshot := n.executor.metrics.Snapshot()
		completedNodes = snapshot.CompletedNodes
		pausedNodes = make([]string, len(snapshot.PausedNodes)+1)
		copy(pausedNodes, snapshot.PausedNodes)
		pausedNodes[len(snapshot.PausedNodes)] = n.nodeName // Add current node to paused
	} else {
		pausedNodes = []string{n.nodeName}
	}

	// Get current superstep from metrics (if available)
	var currentSuperstep int64
	if n.executor.metrics != nil {
		currentSuperstep = n.executor.metrics.Snapshot().CurrentSuperstep
	}

	// Create checkpoint
	chkpt := &checkpoint.Checkpoint{
		RunID:          n.runID,
		Superstep:      currentSuperstep,
		Timestamp:      vsnap.Timestamp,
		Version:        0,
		State:          vsnap.Data,
		PendingWrites:  pendingWrites,
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
		Metadata: map[string]any{
			"interrupt_node":  n.nodeName,
			"interrupt_stage": stage,
		},
	}

	logger.Info("interrupt checkpoint created",
		"node", n.nodeName,
		"stage", stage,
		"pending_writes", len(pendingWrites))

	// Save checkpoint if checkpointer is available
	if n.checkpointer != nil {
		// Use context.WithoutCancel to ensure checkpoint saves even if interrupted
		saveCtx := context.WithoutCancel(ctx)
		if err := n.checkpointer.Save(saveCtx, chkpt); err != nil {
			logger.Error("failed to save interrupt checkpoint",
				"node", n.nodeName,
				"stage", stage,
				"error", err)
		} else {
			logger.Info("interrupt checkpoint saved successfully",
				"node", n.nodeName,
				"stage", stage,
				"run_id", n.runID)
		}
	} else {
		logger.Warn("no checkpointer available, interrupt checkpoint not saved",
			"node", n.nodeName,
			"stage", stage)
	}
}

// Run executes the node.
//
//nolint:gocyclo,nestif // BSP node execution requires complex state management
func (n *pregelNodeAdapter[I, O]) Run(
	ctx context.Context,
	vertex pregel.VertexContext[*Compiled[I, O], state.Updates],
	incoming []pregel.Message[state.Updates],
) error {
	node := n.compiled.graph.Nodes[n.nodeName]
	if node == nil {
		return nil // Skip missing nodes
	}

	// Check if node is already completed or paused (checkpoint resume scenario)
	if n.executor != nil && n.executor.metrics != nil {
		snapshot := n.executor.metrics.Snapshot()

		// Only skip completed nodes if we're resuming from an interrupt (have resuming nodes)
		// This allows normal auto-restore to re-execute from the start
		if len(snapshot.ResumingNodes) > 0 {
			// Skip completed nodes - they already executed before interrupt
			if slices.Contains(snapshot.CompletedNodes, n.nodeName) {
				// Node already completed before interrupt, skip re-execution
				return nil
			}
		}

		// Skip paused nodes - they require external resume signal
		for _, pausedNode := range snapshot.PausedNodes {
			if pausedNode == n.nodeName {
				// Node is paused, wait for external ResumePaused call
				return nil
			}
		}
	}

	// Check for interrupt-before (but skip if we're resuming this node)
	isResuming := false
	if n.executor != nil && n.executor.metrics != nil {
		if slices.Contains(n.executor.metrics.Snapshot().ResumingNodes, n.nodeName) {
			isResuming = true
		}
	}

	if !isResuming && n.shouldInterruptBefore() {
		// Create checkpoint with pending writes (node not executed yet)
		n.createInterruptCheckpoint(ctx, vertex, nil, true)
		// Mark node as paused so it doesn't execute again until resumed
		if n.executor != nil && n.executor.metrics != nil {
			n.executor.metrics.AddPaused(n.nodeName)
		}
		return nil // Pause execution
	}

	// Clear resuming flag after checking (node is now executing)
	if isResuming && n.executor != nil && n.executor.metrics != nil {
		n.executor.metrics.ClearResuming(n.nodeName)
	}

	// Apply incoming state updates from distributed nodes (BSP synchronization)
	if n.enableDistributedState && len(incoming) > 0 {
		for _, msg := range incoming {
			if len(msg.Data) == 0 {
				continue // Routing signal only, no state
			}

			// msg.Data is already state.Updates - apply directly
			if err := n.compiled.manager.ApplyUpdates(ctx, msg.Data); err != nil {
				return fmt.Errorf("failed to apply distributed state updates: %w", err)
			}
		}
	}

	// Create a stream writer that yields intermediate results
	streamWriter := func(intermediateResult state.Updates) {
		if intermediateResult == nil {
			return
		}

		// Extract output from updates based on configured key
		if n.executor.outputKey == "*" {
			// Yield entire state.Updates (for state-only executor)
			output := n.executor.outputAdapter(intermediateResult)
			n.yield(output, nil)
		} else if value, ok := intermediateResult[n.executor.outputKey]; ok {
			// Yield specific key value
			output := n.executor.outputAdapter(value)
			n.yield(output, nil)
		}
	}

	// Attach stream writer to context
	ctxWithStream := WithStreamWriter(ctx, streamWriter)

	// Execute the node with BSP-correct state access
	// Use the superstep-wide ReadView (shared by all nodes in this superstep)
	view := n.graphAdapter.getSuperstepView()
	if view == nil {
		// Fallback for initialization (should not happen in normal operation)
		var err error
		view, err = n.compiled.manager.CreateReadView(ctx)
		if err != nil {
			return fmt.Errorf("failed to create read view: %w", err)
		}
	}

	// Execute node with retry and cache policies
	updates, err := n.executeWithPolicies(ctxWithStream, node, view)
	if err != nil {
		// Wrap node execution errors with sentinel for identification
		return fmt.Errorf("%w: node %q: %w", state.ErrNodeExecution, n.nodeName, err)
	}

	if updates == nil {
		return nil
	}

	// Check for interrupt-after (before applying updates)
	if n.shouldInterruptAfter() {
		// Create checkpoint with pending writes (updates not yet applied)
		n.createInterruptCheckpoint(ctx, vertex, updates, false)
		// Mark node as paused so execution doesn't continue
		if n.executor != nil && n.executor.metrics != nil {
			n.executor.metrics.AddPaused(n.nodeName)
		}
		return nil // Pause execution without applying updates
	}

	// Track node completion for checkpoint metadata
	if n.executor != nil && n.executor.metrics != nil {
		n.executor.metrics.AddCompleted(n.nodeName)
	}

	// Apply updates immediately for routing decisions within the same node
	// BSP semantics are maintained because other nodes use the superstep snapshot
	if len(updates) > 0 {
		if err := n.compiled.manager.ApplyUpdates(ctx, updates); err != nil {
			return fmt.Errorf("failed to apply state updates: %w", err)
		}

		// Extract output from updates based on configured key and yield
		if n.executor.outputKey == "*" {
			// Yield entire state.Updates (for state-only executor)
			output := n.executor.outputAdapter(updates)
			if !n.yield(output, nil) {
				return nil
			}
		} else if value, ok := updates[n.executor.outputKey]; ok {
			// Special handling for slices: unfold and yield each element individually
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
	var stateData state.Updates
	if n.enableDistributedState && len(updates) > 0 {
		stateData = updates
	}

	// Check for conditional edges first
	condEdges := n.compiled.topology.conditionalByFrom
	found := false
	for idx, conditions := range condEdges {
		for _, cond := range conditions {
			if cond.From == n.nodeName {
				found = true
				// CONDITIONAL ROUTING SEMANTICS:
				// Creates fresh snapshot to evaluate routing - sees node's own updates.
				//
				// What routing sees:
				//   - State from superstep N-1 (last superstep's final state)
				//   - This node's updates from superstep N (applied immediately at line 599)
				//   - NOT other nodes' updates from superstep N (BSP isolation)
				//
				// This violates pure BSP but enables agent patterns (ReAct, tool routing).
				// Example: Model node produces AIMessage with tool_calls → routing checks
				// tool_calls → routes to tool executor. Routing MUST see the AIMessage.
				//
				// Trade-off:
				// - Pure BSP: Routing uses superstep snapshot → can't see own output → agents break
				// - Hybrid: Routing sees own updates → agent routing works → not pure BSP
				condView, err := n.compiled.manager.CreateReadView(ctx)
				if err != nil {
					return fmt.Errorf("failed to create read view for conditionals: %w", err)
				}
				targets := cond.Condition(ctx, condView)
				for _, target := range targets {
					if target != EndNode {
						vertex.Send(pregel.Message[state.Updates]{
							From: n.nodeName,
							To:   target,
							Data: stateData,
						})
					}
				}
				_ = idx // Mark as used
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		// Use regular outgoing edges
		for _, target := range vertex.State.topology.outgoing[n.nodeName] {
			if target != EndNode {
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
