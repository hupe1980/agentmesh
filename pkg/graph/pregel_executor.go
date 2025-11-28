package graph

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"runtime"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/internal/chanutil"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

const (
	// defaultResultChanSize buffers up to 100 results to prevent backpressure
	// when the yield consumer (caller iterating results) is slower than the
	// producer (graph execution). This provides:
	//   - Smoother execution flow without blocking nodes
	//   - Tolerance for bursty output patterns
	//   - Balance between memory usage (~8KB per channel) and throughput
	// For workloads with >100 concurrent results, execution may block briefly
	// until the consumer catches up. Typical agents produce <10 results/superstep.
	defaultResultChanSize = 100

	// defaultMaxIterations is the default maximum number of supersteps (iterations)
	// before execution terminates to prevent infinite loops. This provides:
	//   - Protection against buggy graphs with infinite routing cycles
	//   - Reasonable default for most agent workflows (simple: <10, complex: <100)
	//   - Can be overridden via WithMaxIterations() for specialized workloads
	// Agents typically complete in 5-20 iterations. Exceeding 1000 often indicates
	// a routing bug (e.g., node always routes back to itself without termination).
	defaultMaxIterations = 1000
)

// unfoldValue attempts to unfold a value if it's a slice, yielding each element.
// Returns true if the value was a slice and was unfolded, false otherwise.
// This replaces reflection-based slice handling with type assertions for common types.
func unfoldValue(value any, yield func(any) bool) bool {
	// Check for SliceValue interface first (handles state.SliceOf[T] from UpdateBuilder)
	if sv, ok := value.(state.SliceValue); ok {
		slice := sv.ToSlice()
		return yieldSlice(slice, yield)
	}

	// Try common slice types (most frequent cases)
	switch v := value.(type) {
	case []message.Message:
		return yieldSlice(v, yield)
	case []any:
		return yieldSlice(v, yield)
	case []string:
		return yieldSlice(v, yield)
	case []int:
		return yieldSlice(v, yield)
	case []float64:
		return yieldSlice(v, yield)
	default:
		// Not a recognized slice type - return false to process as single value
		// If you need to unfold a different slice type, add it explicitly above
		// for optimal performance (reflection is 10-100x slower)
		return false
	}
}

// yieldSlice is a generic helper that yields each element of a slice.
// Returns true to indicate the slice was processed.
func yieldSlice[T any](slice []T, yield func(any) bool) bool {
	for _, elem := range slice {
		if !yield(elem) {
			return true
		}
	}
	return true
}

// PregelExecutor is a Bulk-Synchronous Parallel (BSP) executor using the pregel runtime.
// This is the default executor for agent workflows and chat systems.
//
// # BSP Semantics with Command Pattern
//
// AgentMesh implements pure BSP semantics using the Command pattern:
//
// BSP Execution Model:
//   - Inter-node isolation: Nodes read from shared superstep snapshot (read-only view)
//   - Superstep barriers: All nodes finish before next superstep begins
//   - Distributed state: Updates propagated via messages, delivered next superstep
//   - Aggregators: Thread-safe accumulation across nodes
//
// Command Pattern for Routing:
//   - Nodes receive read-only state snapshot (BSP-compliant)
//   - Nodes compute results and routing decision atomically
//   - Return Command{Updates, Goto} - both determined from snapshot
//   - Updates applied after node execution, visible next superstep
//
// Agent Workflow Example (ReAct):
//
//	Model node reads messages from snapshot → generates AIMessage → returns:
//	  Command{
//	    Updates: {"messages": append(msgs, aiMsg)},
//	    Goto: aiMsg.HasToolCalls() ? []string{"tool"} : []string{EndNode}
//	  }
//
// The routing decision is made within the node logic using the snapshot state,
// not by evaluating fresh state after updates. This maintains pure BSP semantics.
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

	// Checkpoint resume state (set during checkpoint restore)
	resumeEntryPoints []string // Nodes to execute when resuming from checkpoint
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
		maxIters:      defaultMaxIterations,
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

// initializeRun handles run initialization: resume values, checkpoint restore, and initial state.
func (p *PregelExecutor[I, O]) initializeRun(
	ctx context.Context,
	compiled *Compiled[I, O],
	input I,
	opts RunOptions,
) (string, error) {
	// Validate: RunID is required when checkpointing is enabled
	// This prevents silent bypass of checkpoint/resume logic when auto-generated UUIDs
	// would create new checkpoint streams instead of resuming existing ones.
	// Exception: When resuming with WithCheckpoint, the RunID comes from the checkpoint.
	if opts.Checkpointer != nil && opts.RunID == "" && opts.Checkpoint == nil {
		return "", ErrRunIDRequired
	}

	// Inject resume values into context if provided
	if opts.ResumeValue != nil {
		ctx = withResumeValueContext(ctx, opts.ResumeValue)
	}

	// Determine runID: explicit > checkpoint > auto-generated
	runID := opts.RunID
	if runID == "" && opts.Checkpoint != nil {
		runID = opts.Checkpoint.RunID
	}
	if runID == "" {
		runID = uuid.New().String()
	}

	// Restore from checkpoint if configured
	if err := p.restoreCheckpoint(ctx, compiled, opts); err != nil {
		return "", err
	}

	// Convert input to initial state using adapter
	initialState := p.inputToState(input)

	if len(initialState) > 0 {
		if err := compiled.manager.ApplyUpdates(ctx, initialState); err != nil {
			return "", fmt.Errorf("%w: initial state: %w", ErrStateApply, err)
		}
	}

	return runID, nil
}

// setupYieldChannel creates the result channel and yield goroutine for thread-safe result delivery.
func (p *PregelExecutor[I, O]) setupYieldChannel(
	ctx context.Context,
	yield func(O, error) bool,
	cancel context.CancelFunc,
) (chan struct {
	output O
	err    error
}, chan struct{}, func(O, error) bool) {
	resultChan := make(chan struct {
		output O
		err    error
	}, defaultResultChanSize)

	yieldDone := make(chan struct{})
	go func() {
		defer close(yieldDone)
		for {
			select {
			case <-ctx.Done():
				// Context cancelled - drain channel and exit
				chanutil.DrainUntilClosed(resultChan)
				return
			case item, ok := <-resultChan:
				if !ok {
					return // Channel closed
				}
				if !yield(item.output, item.err) {
					cancel()
					chanutil.DrainUntilClosed(resultChan)
					return
				}
			}
		}
	}()

	safeYield := func(output O, err error) bool {
		select {
		case resultChan <- struct {
			output O
			err    error
		}{output, err}:
			return true
		case <-ctx.Done():
			return false
		}
	}

	return resultChan, yieldDone, safeYield
}

// buildRuntimeOptions constructs pregel runtime options with BSP callbacks.
func (p *PregelExecutor[I, O]) buildRuntimeOptions(
	compiled *Compiled[I, O],
	runOpts RunOptions,
	adapter *pregelGraphAdapter[I, O],
) []pregel.RuntimeOption[*Compiled[I, O], state.Updates] {
	runtimeOpts := []pregel.RuntimeOption[*Compiled[I, O], state.Updates]{
		pregel.WithMaxWorkers[*Compiled[I, O], state.Updates](p.maxWorkers),
		pregel.WithMaxIterations[*Compiled[I, O], state.Updates](p.maxIters),
	}

	if p.messageBus != nil {
		runtimeOpts = append(runtimeOpts, pregel.WithMessageBus[*Compiled[I, O]](p.messageBus))
	}

	if len(p.aggregators) > 0 {
		runtimeOpts = append(runtimeOpts, pregel.WithAggregators[*Compiled[I, O], state.Updates](p.aggregators))
	}

	// BSP barrier callbacks
	runtimeOpts = append(runtimeOpts,
		pregel.WithOnSuperstepStart[*Compiled[I, O], state.Updates](
			func(ctx context.Context, superstep int64, frontier pregel.FrontierInfo) error {
				// Publish superstep start event with frontier diagnostics
				Publish(ctx, Event{
					Type:      EventSuperstepStart,
					Superstep: int(superstep),
					Timestamp: time.Now(),
					Data: map[string]any{
						"frontier_size":  frontier.Size,
						"frontier_nodes": frontier.Nodes,
					},
				})
				return adapter.prepareSuperstep(ctx)
			},
		),
		pregel.WithOnSuperstepComplete[*Compiled[I, O], state.Updates](
			func(ctx context.Context, superstep int64) error {
				// Handle checkpoint/updates first
				if runOpts.Checkpointer != nil && runOpts.RunID != "" {
					if err := p.saveCheckpoint(ctx, compiled, runOpts, superstep, adapter); err != nil {
						return err
					}
				} else {
					p.applyPendingUpdates(ctx, compiled, adapter)
				}

				// Publish superstep complete event
				Publish(ctx, Event{
					Type:      EventSuperstepComplete,
					Superstep: int(superstep),
					Timestamp: time.Now(),
					Data:      map[string]any{},
				})
				return nil
			},
		),
	)

	return runtimeOpts
}

// executeRuntimeLoop runs the pregel runtime and handles events, errors, and final state.
func (p *PregelExecutor[I, O]) executeRuntimeLoop(
	ctx context.Context,
	rt *pregel.Runtime[*Compiled[I, O], state.Updates],
	compiled *Compiled[I, O],
	runOpts RunOptions,
	adapter *pregelGraphAdapter[I, O],
	safeYield func(O, error) bool,
	resultChan chan struct {
		output O
		err    error
	},
	yieldDone chan struct{},
) {
	var runtimeErr error

	// Use defer to always publish completion event, regardless of how we exit
	defer func() {
		// Publish completion event based on final error state
		if runtimeErr == nil {
			finalSuperstep := rt.CurrentSuperstep()
			Publish(ctx, Event{
				Type:      EventGraphComplete,
				Timestamp: time.Now(),
				Data: map[string]any{
					"run_id":    runOpts.RunID,
					"superstep": finalSuperstep,
				},
			})
		} else {
			Publish(ctx, Event{
				Type:      EventGraphError,
				Timestamp: time.Now(),
				Data: map[string]any{
					"run_id": runOpts.RunID,
					"error":  runtimeErr.Error(),
				},
			})
		}

		// Events are delivered synchronously, no sleep needed
		close(resultChan)
		<-yieldDone
	}()

	eventCount := 0
	for pregelEvent, err := range rt.Run(ctx) {
		eventCount++
		if p.metrics != nil {
			p.metrics.SetSuperstep(rt.CurrentSuperstep())
		}

		// Publish graph events for visualization
		if pregelEvent.Vertex != "" {
			// Check if this is a start event (marked by special output value)
			if pregelEvent.Output == "__vertex_start__" {
				// Publish node start event
				Publish(ctx, Event{
					Type:      EventNodeStart,
					Node:      pregelEvent.Vertex,
					Superstep: int(pregelEvent.Superstep),
					Timestamp: time.Now(),
					Data:      map[string]any{},
				})
			} else {
				// Publish node complete event
				Publish(ctx, Event{
					Type:      EventNodeComplete,
					Node:      pregelEvent.Vertex,
					Superstep: int(pregelEvent.Superstep),
					Timestamp: time.Now(),
					Data: map[string]any{
						"output": pregelEvent.Output,
					},
				})
			}
		}
		// Events without vertex (errors, cancellations) are not published as node events

		if err != nil {
			// Only treat as fatal error if it's not just context cancellation
			// Context cancellation is expected during shutdown
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				runtimeErr = err
				var zero O
				safeYield(zero, err)
				break
			}
			// Ignore context cancellation - let the loop exit naturally
		}

		// Check for cancellation between events
		// Don't treat normal shutdown cancellation as an error
		select {
		case <-ctx.Done():
			// Context cancelled - this is normal during shutdown
			// Let the loop exit naturally and defer will handle cleanup
			return
		default:
		}
	}

	// Handle final state
	finalSuperstep := rt.CurrentSuperstep()
	p.applyPendingUpdates(ctx, compiled, adapter)
	if runtimeErr == nil && runOpts.Checkpointer != nil && runOpts.RunID != "" {
		p.saveFinalCheckpoint(ctx, compiled, runOpts, finalSuperstep)
	}

	// Completion event is published by defer at function exit
}

// Run executes the compiled graph using the pregel BSP runtime.
func (p *PregelExecutor[I, O]) Run(
	ctx context.Context,
	compiled *Compiled[I, O],
	input I,
	opts ...RunOption) iter.Seq2[O, error] {
	return func(yield func(O, error) bool) {
		// Extract run options
		runOpts := ApplyOptions(opts...)

		// Create a cancellable context so we can stop the runtime when consumer exits
		// Note: We don't defer cancel() here because executeRuntimeLoop needs the context
		// to remain valid for publishing completion events. The cancel() is only called
		// by setupYieldChannel when the consumer stops consuming (yield returns false).
		runCtx, cancel := context.WithCancel(ctx)

		// Inject approval responses into context if provided
		if len(runOpts.Approvals) > 0 {
			for nodeName, approval := range runOpts.Approvals {
				runCtx = WithApprovalResponse(runCtx, nodeName, approval)
			}
		}

		// Initialize run context and state
		runID, err := p.initializeRun(runCtx, compiled, input, runOpts)
		if err != nil {
			cancel() // Cancel on init error
			var zero O
			yield(zero, err)
			return
		}

		// Setup result channel and safe yielding
		// The cancel function will be called by the yield goroutine if consumer stops
		resultChan, yieldDone, safeYield := p.setupYieldChannel(runCtx, yield, cancel)

		// Create adapter to make compiled graph work with pregel runtime
		adapter := &pregelGraphAdapter[I, O]{
			compiled:               compiled,
			runID:                  runID,
			yield:                  safeYield,
			enableDistributedState: p.enableDistributedState,
			executor:               p,
			checkpointer:           runOpts.Checkpointer, // For interrupt checkpoints
		}

		// Configure pregel runtime with callbacks
		runtimeOpts := p.buildRuntimeOptions(compiled, runOpts, adapter)

		// Create and run the pregel runtime
		rt, err := pregel.NewRuntime(adapter, runtimeOpts...)
		if err != nil {
			var zero O
			safeYield(zero, err)
			close(resultChan)
			<-yieldDone
			return
		}

		// Publish graph start event
		Publish(runCtx, Event{
			Type:      EventGraphStart,
			Timestamp: time.Now(),
			Data: map[string]any{
				"run_id": runID,
			},
		})

		// Execute runtime and handle completion
		p.executeRuntimeLoop(runCtx, rt, compiled, runOpts, adapter, safeYield, resultChan, yieldDone)

		// Cancel context after execution completes
		// This is safe even if cancel() was already called by the yield goroutine
		cancel()
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
	currentSuperstepView state.ReadView
	mu                   sync.RWMutex

	// Track which nodes executed in previous superstep for trigger-based optimization
	// Only nodes triggered by previously executed nodes will run in subsequent supersteps
	executedNodes map[string]bool

	// Two-phase commit support: collect updates from all nodes in superstep
	// before applying them (after checkpoint save)
	pendingUpdates []checkpoint.PendingWrite
	updatesMu      sync.Mutex
}

// RootVertices returns vertices that should execute in the current superstep.
// Uses trigger-based optimization to avoid unnecessary vertex execution:
// - Checkpoint resume: explicit resume entry points from checkpoint
// - Paused resume: nodes that were explicitly paused (human-in-loop)
// - First superstep: nodes directly connected from START
// - Subsequent supersteps: nodes triggered by previously executed nodes
func (a *pregelGraphAdapter[I, O]) RootVertices() []string {
	// PRIORITY 1: Checkpoint resume with explicit entry points
	// This is the new, explicit mechanism that tells us exactly where to resume
	if a.executor != nil && len(a.executor.resumeEntryPoints) > 0 {
		resumePoints := a.executor.resumeEntryPoints
		a.executor.resumeEntryPoints = nil // Clear after first use
		return resumePoints
	}

	// PRIORITY 2: Paused node resume (human-in-loop workflows)
	// If resuming from a pause, return the resuming nodes as roots
	if a.executor != nil && a.executor.metrics != nil {
		snapshot := a.executor.metrics.Snapshot()
		if len(snapshot.ResumingNodes) > 0 {
			// When resuming from pause, start execution from the paused nodes
			return snapshot.ResumingNodes
		}
	}

	a.mu.RLock()
	executedNodes := a.executedNodes
	a.mu.RUnlock()

	// PRIORITY 3: First superstep - return nodes directly connected from START
	if len(executedNodes) == 0 {
		outgoing := a.compiled.topology.outgoing[StartNode]
		if len(outgoing) > 0 {
			return outgoing
		}
		return []string{}
	}

	// Subsequent supersteps: return nodes triggered by previously executed nodes
	// This optimization ensures only nodes with incoming data/signals will execute
	triggered := make(map[string]bool)
	for nodeName := range executedNodes {
		// Find nodes that can be triggered by this node's execution
		if targets := a.compiled.topology.triggerToNodes[nodeName]; len(targets) > 0 {
			for _, target := range targets {
				triggered[target] = true
			}
		}
	}

	// Convert to sorted slice for deterministic execution order
	result := make([]string, 0, len(triggered))
	for node := range triggered {
		result = append(result, node)
	}
	sort.Strings(result)
	return result
}

// Outgoing returns outgoing edges for a vertex.
func (a *pregelGraphAdapter[I, O]) Outgoing(vertexName string) []string {
	return a.compiled.topology.outgoing[vertexName]
}

// VertexByName returns a pregel vertex adapter.
func (a *pregelGraphAdapter[I, O]) VertexByName(name string) pregel.Vertex[*Compiled[I, O], state.Updates] {
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
		return fmt.Errorf("%w: %w", ErrSnapshotCreate, err)
	}

	a.currentSuperstepView = view

	// Clear executed nodes from previous superstep to prepare for tracking this superstep
	// The nodes that execute in this superstep will be recorded and used to determine
	// which nodes should run in the NEXT superstep (trigger-based optimization)
	a.executedNodes = make(map[string]bool)

	return nil
}

// getSuperstepView returns the read-only view for the current superstep.
func (a *pregelGraphAdapter[I, O]) getSuperstepView() state.ReadView {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentSuperstepView
}

// pregelNodeAdapter adapts a graph.Node to pregel.Vertex interface.
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
	view state.ReadView,
) ([]string, state.Updates, error) {
	// Priority 1: Check if node implements NodeWithRetry interface (modern approach)
	var retryPolicy *RetryPolicy
	if retryNode, ok := node.(NodeWithRetry); ok {
		retryPolicy = retryNode.RetryPolicy()
	}

	// Priority 2: Fall back to NodeConfigs map (legacy approach via WithRetryPolicy option)
	if retryPolicy == nil {
		config := n.compiled.graph.NodeConfigs[n.nodeName]
		if config == nil {
			config = defaultNodeConfig()
		}
		retryPolicy = config.RetryPolicy
	}

	// Execute with retry policy
	return n.executeWithRetry(ctx, node, view, retryPolicy)
}

// executeWithRetry executes a node with retry policy applied.
func (n *pregelNodeAdapter[I, O]) executeWithRetry(
	ctx context.Context,
	node Node,
	view state.ReadView,
	policy *RetryPolicy,
) ([]string, state.Updates, error) {
	// Execute node with retry logic (now returns tuple)
	return n.executeNodeWithPolicy(ctx, node, view, policy)
}

// executeBeforeNodeCallback executes BeforeNode plugin callback.
// Returns short-circuit tuple (targets, updates, error) if callback provides one.
// executeNodeWithPolicy executes the node with retry policy.
// For NamespacedNodes, enforcement happens through namespaced keys -
// the CommandFunc should only use keys from its declared namespace.
func (n *pregelNodeAdapter[I, O]) executeNodeWithPolicy(
	ctx context.Context,
	node Node,
	view state.ReadView,
	policy *RetryPolicy,
) ([]string, state.Updates, error) {
	if policy == nil || policy.MaxAttempts <= 1 {
		return node.Execute(ctx, view)
	}

	return n.executeWithRetryLoop(ctx, node, view, policy)
}

// executeWithRetryLoop executes node with retry attempts and backoff.
func (n *pregelNodeAdapter[I, O]) executeWithRetryLoop(
	ctx context.Context,
	node Node,
	view state.ReadView,
	policy *RetryPolicy,
) ([]string, state.Updates, error) {
	var targets []string
	var updates state.Updates
	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		targets, updates, lastErr = node.Execute(ctx, view)
		if lastErr == nil {
			return targets, updates, nil
		}

		// Check if error is retryable
		if policy.Retryable != nil && !policy.Retryable(lastErr) {
			break
		}

		// Apply backoff before next attempt
		if attempt < policy.MaxAttempts && policy.Backoff != nil {
			if err := n.applyBackoff(ctx, policy, attempt); err != nil {
				return nil, nil, err
			}
		}
	}

	if policy.MaxAttempts > 1 {
		return nil, nil, &retryExceededError{
			sentinel:    ErrRetryExceeded,
			maxAttempts: policy.MaxAttempts,
			lastErr:     lastErr,
		}
	}
	return nil, nil, lastErr
}

// applyBackoff waits for backoff duration or context cancellation.
func (n *pregelNodeAdapter[I, O]) applyBackoff(ctx context.Context, policy *RetryPolicy, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(policy.Backoff(attempt)):
		return nil
	}
}

// executeAfterNodeCallback executes AfterNode or OnNodeError plugin callback.

// evaluateApprovalGuard evaluates the approval guard for a node.
// Returns (needsApproval, reason) if successful, or (true, error message) if guard fails.
func (n *pregelNodeAdapter[I, O]) evaluateApprovalGuard(ctx context.Context, view state.ReadView) (bool, string) {
	config, ok := n.compiled.graph.ApprovalConfigs[n.nodeName]
	if !ok || config.Guard == nil {
		// No guard configured - always interrupt (legacy behavior)
		return true, ""
	}

	needsApproval, reason, err := config.Guard(ctx, view)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Error("approval guard evaluation failed", "node", n.nodeName, "error", err)
		// On guard error, default to requiring approval for safety
		return true, fmt.Sprintf("guard evaluation failed: %v", err)
	}
	return needsApproval, reason
}

// shouldInterruptBefore checks if this node needs approval before execution.
// Returns true if the node requires interrupt, along with optional approval reason.
func (n *pregelNodeAdapter[I, O]) shouldInterruptBefore(ctx context.Context, view state.ReadView) (bool, string) {
	if !slices.Contains(n.compiled.graph.InterruptBefore, n.nodeName) {
		return false, ""
	}
	return n.evaluateApprovalGuard(ctx, view)
}

// shouldInterruptAfter checks if this node needs approval after execution.
// Returns true if the node requires interrupt, along with optional approval reason.
func (n *pregelNodeAdapter[I, O]) shouldInterruptAfter(ctx context.Context, view state.ReadView) (bool, string) {
	if !slices.Contains(n.compiled.graph.InterruptAfter, n.nodeName) {
		return false, ""
	}
	return n.evaluateApprovalGuard(ctx, view)
}

// getInterruptStage returns the stage name for the interrupt.
func (n *pregelNodeAdapter[I, O]) getInterruptStage(isBefore bool) string {
	if isBefore {
		return "before"
	}
	return "after"
}

// getRequiredState extracts required state fields based on approval configuration.
func (n *pregelNodeAdapter[I, O]) getRequiredState(vsnap *state.VersionedSnapshot) state.Updates {
	config, ok := n.compiled.graph.ApprovalConfigs[n.nodeName]
	if !ok {
		return nil
	}

	if len(config.StateSnapshot) > 0 {
		// Filter state to only include requested keys
		requiredState := make(state.Updates)
		for _, key := range config.StateSnapshot {
			if value, exists := vsnap.Data[key]; exists {
				requiredState[key] = value
			}
		}
		return requiredState
	}

	// Empty list means include all state
	return vsnap.Data
}

// createApprovalMetadata creates approval metadata for an interrupt checkpoint.
func (n *pregelNodeAdapter[I, O]) createApprovalMetadata(vsnap *state.VersionedSnapshot, approvalReason string) *checkpoint.ApprovalMetadata {
	// Get timeout from approval configuration (if configured)
	var timeoutAt *time.Time
	if config, ok := n.compiled.graph.ApprovalConfigs[n.nodeName]; ok && config.Timeout > 0 {
		timeout := time.Now().Add(config.Timeout)
		timeoutAt = &timeout
	}

	// Get required state snapshot (if configured)
	requiredState := n.getRequiredState(vsnap)

	return &checkpoint.ApprovalMetadata{
		PendingApprovals: map[string]*checkpoint.PendingApproval{
			n.nodeName: {
				NodeName:      n.nodeName,
				Reason:        approvalReason,
				RequestedAt:   vsnap.Timestamp,
				TimeoutAt:     timeoutAt,
				RequiredState: requiredState,
			},
		},
		ApprovalHistory: []checkpoint.ApprovalRecord{},
		GuardReasons:    map[string]string{n.nodeName: approvalReason},
	}
}

// createStateSnapshot creates a state snapshot for the interrupt.
func (n *pregelNodeAdapter[I, O]) createStateSnapshot(ctx context.Context, stage string) (*state.VersionedSnapshot, error) {
	return n.compiled.manager.Snapshot(ctx, map[string]string{
		"run_id":    n.runID,
		"node":      n.nodeName,
		"interrupt": stage,
	})
}

// createInterruptCheckpoint creates a checkpoint with pending writes and pauses execution.
func (n *pregelNodeAdapter[I, O]) createInterruptCheckpoint(
	ctx context.Context,
	updates state.Updates,
	isBefore bool,
	approvalReason string,
) {
	if n.executor == nil {
		return // No executor, can't create checkpoint
	}

	logger := logging.FromContext(ctx)
	stage := n.getInterruptStage(isBefore)

	logger.Info("interrupt detected, creating checkpoint",
		"node", n.nodeName,
		"stage", stage,
		"approval_reason", approvalReason)

	// Get current state snapshot
	vsnap, err := n.createStateSnapshot(ctx, stage)
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

	// Create approval metadata
	approvalMetadata := n.createApprovalMetadata(vsnap, approvalReason)

	// Create checkpoint
	chkpt := &checkpoint.Checkpoint{
		RunID:            n.runID,
		Superstep:        currentSuperstep,
		Timestamp:        vsnap.Timestamp,
		Version:          0,
		State:            vsnap.Data,
		PendingWrites:    pendingWrites,
		CompletedNodes:   completedNodes,
		PausedNodes:      pausedNodes,
		ApprovalMetadata: approvalMetadata,
		Metadata: map[string]any{
			"interrupt_node":  n.nodeName,
			"interrupt_stage": stage,
		},
	}

	logger.Info("interrupt checkpoint created",
		"node", n.nodeName,
		"stage", stage,
		"pending_writes", len(pendingWrites),
		"approval_required", approvalReason != "")

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

// notifyStateChangeCallbacks executes state change callbacks if available.
func (n *pregelNodeAdapter[I, O]) notifyStateChangeCallbacks(
	ctx context.Context,
	updates state.Updates,
	logger logging.Logger,
) {
	// No-op: state change notifications now handled by middleware pattern
}

// yieldOutputFromUpdates extracts output from updates and yields it.
// Returns false if yielding was cancelled, true otherwise.
func (n *pregelNodeAdapter[I, O]) yieldOutputFromUpdates(updates state.Updates) bool {
	// Yield entire state.Updates for wildcard output key
	if n.executor.outputKey == "*" {
		output := n.executor.outputAdapter(updates)
		return n.yield(output, nil)
	}

	// Yield specific key value
	value, ok := updates[n.executor.outputKey]
	if !ok {
		return true // Key not in updates, continue
	}

	// Try to unfold slices and yield each element
	wasUnfolded := unfoldValue(value, func(elem any) bool {
		output := n.executor.outputAdapter(elem)
		return n.yield(output, nil)
	})

	if wasUnfolded {
		return true // Slice was unfolded and yielded
	}

	// For non-slice values, apply adapter and yield once
	output := n.executor.outputAdapter(value)
	return n.yield(output, nil)
}

// Run executes the node.
//
//nolint:gocyclo // BSP node execution requires complex state management
func (n *pregelNodeAdapter[I, O]) Run(
	ctx context.Context,
	vertex pregel.VertexContext[*Compiled[I, O], state.Updates],
	incoming []pregel.Message[state.Updates],
) error {
	node := n.compiled.graph.Nodes[n.nodeName]
	if node == nil {
		return nil // Skip missing nodes
	}

	// Check if node is paused (waiting for external resume signal)
	// Note: Completed node skipping is now handled by RootVertices() via resumeEntryPoints,
	// so we only need to check for paused nodes here.
	if n.executor != nil && n.executor.metrics != nil {
		snapshot := n.executor.metrics.Snapshot()

		// Skip paused nodes - they require external resume signal
		if slices.Contains(snapshot.PausedNodes, n.nodeName) {
			// Node is paused, wait for external ResumePaused call
			return nil
		}
	}

	// Observability: Create node-level span with attributes
	tp := trace.FromContext(ctx)
	tracer := tp.Tracer("agentmesh.graph")
	ctx, nodeSpan := tracer.Start(ctx, "node.execute", trace.Attr{Key: "node.name", Value: n.nodeName})
	var nodeErr error
	defer func() {
		nodeSpan.End(nodeErr)
	}()

	// Observability: Log node execution start
	logger := logging.FromContext(ctx)
	logger.Debug("node execution starting", "node", n.nodeName)

	// Observability: Record node execution metrics
	mp := metrics.FromContext(ctx)
	nodeStartTime := time.Now()
	nodeExecCounter := mp.Counter("node.executions")
	nodeExecCounter.Add(ctx, 1, metrics.Attr{Key: "node", Value: n.nodeName})
	defer func() {
		duration := time.Since(nodeStartTime)
		nodeDuration := mp.Histogram("node.duration_ms")
		nodeDuration.Record(ctx, float64(duration.Milliseconds()),
			metrics.Attr{Key: "node", Value: n.nodeName})

		if nodeErr != nil {
			// Record error metric
			nodeErrors := mp.Counter("node.errors")
			nodeErrors.Add(ctx, 1, metrics.Attr{Key: "node", Value: n.nodeName})
			logger.Error("node execution failed", "node", n.nodeName, "error", nodeErr, "duration_ms", duration.Milliseconds())
		} else {
			logger.Debug("node execution completed", "node", n.nodeName, "duration_ms", duration.Milliseconds())
		}
	}()

	// Check for interrupt-before (but skip if we're resuming this node)
	isResuming := false
	if n.executor != nil && n.executor.metrics != nil {
		if slices.Contains(n.executor.metrics.Snapshot().ResumingNodes, n.nodeName) {
			isResuming = true
		}
	}

	// Get state view early for approval guard evaluation
	view := n.graphAdapter.getSuperstepView()
	if view == nil {
		// Fallback for initialization
		var err error
		view, err = n.compiled.manager.CreateReadView(ctx)
		if err != nil {
			nodeErr = fmt.Errorf("%w: %w", ErrSnapshotCreate, err)
			return nodeErr
		}
	}

	if !isResuming {
		if needsInterrupt, reason := n.shouldInterruptBefore(ctx, view); needsInterrupt {
			// Create checkpoint with approval metadata
			n.createInterruptCheckpoint(ctx, nil, true, reason)
			// Mark node as paused so it doesn't execute again until resumed
			if n.executor != nil && n.executor.metrics != nil {
				n.executor.metrics.AddPaused(n.nodeName)
			}
			return nil // Pause execution
		}
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
				return fmt.Errorf("%w: %w", ErrDistributedState, err)
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

	// Execute node with retry and cache policies (now returns tuple)
	// Note: view was already created earlier for approval guard evaluation
	targets, updates, err := n.executeWithPolicies(ctxWithStream, node, view)
	if err != nil {
		// Wrap node execution errors with structured error type
		nodeErr = &NodeExecutionError{
			NodeName: n.nodeName,
			Err:      err,
		}
		return nodeErr
	}

	if targets == nil {
		return nil
	}

	// Validate routing decision
	if len(targets) == 0 {
		nodeErr = fmt.Errorf("%w: node %s must specify targets (use graph.EndNode to terminate)", ErrRoutingTargets, n.nodeName)
		return nodeErr
	}

	// Check for interrupt-after (before applying updates)
	if needsInterrupt, reason := n.shouldInterruptAfter(ctx, view); needsInterrupt {
		// Create checkpoint with pending writes (updates not yet applied) and approval metadata
		n.createInterruptCheckpoint(ctx, updates, false, reason)
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

	// Track node execution for trigger-based optimization
	// Records which nodes executed so only their downstream targets run in next superstep
	n.graphAdapter.mu.Lock()
	if n.graphAdapter.executedNodes == nil {
		n.graphAdapter.executedNodes = make(map[string]bool)
	}
	n.graphAdapter.executedNodes[n.nodeName] = true
	n.graphAdapter.mu.Unlock()

	// Collect updates for two-phase commit (defer application until after checkpoint save)
	// These updates will be applied at superstep completion, after checkpoint is saved
	// This ensures transactional semantics: if crash happens, checkpoint has pending writes
	if len(updates) > 0 {
		n.graphAdapter.updatesMu.Lock()
		timestamp := time.Now()
		for channel, value := range updates {
			n.graphAdapter.pendingUpdates = append(n.graphAdapter.pendingUpdates, checkpoint.PendingWrite{
				NodeName:  n.nodeName,
				Channel:   channel,
				Value:     value,
				Timestamp: timestamp,
			})
		}
		n.graphAdapter.updatesMu.Unlock()

		// Notify state change callbacks (before actual application)
		n.notifyStateChangeCallbacks(ctx, updates, logger)

		// Extract output from updates based on configured key and yield
		if !n.yieldOutputFromUpdates(updates) {
			return nil
		}
	}

	// Send routing signals (and optionally state) to next nodes via pregel runtime
	// Use routing targets from tuple instead of edges/conditional edges
	var stateData state.Updates
	if n.enableDistributedState && updates != nil && len(updates) > 0 {
		stateData = updates
	}

	// Use routing targets from tuple (unified routing model)
	// The routing targets come from the tuple returned by the node
	for _, target := range targets {
		if target != EndNode {
			// Send message to target node
			vertex.Send(pregel.Message[state.Updates]{
				From: n.nodeName,
				To:   target,
				Data: stateData,
			})
		}
		// If target is EndNode, node execution terminates (no message sent)
	}

	return nil
}
