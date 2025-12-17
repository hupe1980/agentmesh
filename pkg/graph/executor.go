package graph

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"sort"
	"time"

	"github.com/hupe1980/agentmesh/internal/chanutil"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
)

// checkpointRestoreResult contains restored state and pending writes from checkpoint.
type checkpointRestoreResult struct {
	State         map[string]any
	stateOwned    bool
	PendingWrites []checkpoint.PendingWrite // Only set if Committed=false
	ManagedValues []checkpoint.ManagedValueDescriptor
	PausedNodes   []string // Nodes that were paused (waiting for approval)
}

func (r *checkpointRestoreResult) useCheckpoint(cp *checkpoint.Checkpoint) {
	if cp == nil {
		return
	}

	if cp.State != nil {
		r.State = cp.State
		r.stateOwned = false
	} else {
		r.State = nil
		r.stateOwned = true
	}

	if !cp.Committed && len(cp.PendingWrites) > 0 {
		r.PendingWrites = clonePendingWrites(cp.PendingWrites)
	} else {
		r.PendingWrites = nil
	}

	if len(cp.ManagedValues) > 0 {
		r.ManagedValues = cloneManagedValueDescriptors(cp.ManagedValues)
	} else {
		r.ManagedValues = nil
	}

	// Capture paused nodes for resume
	if len(cp.PausedNodes) > 0 {
		r.PausedNodes = append([]string(nil), cp.PausedNodes...)
	} else {
		r.PausedNodes = nil
	}
}

func (r *checkpointRestoreResult) ensureStateOwned() {
	if r.State == nil {
		r.State = make(map[string]any)
		r.stateOwned = true
		return
	}
	if r.stateOwned {
		return
	}
	cloned := maps.Clone(r.State)
	r.State = cloned
	r.stateOwned = true
}

// reduceValue merges a new value into the existing state using the provided reducer.
// This is the unified approach - the reducer determines merge semantics (replace, append, sum, etc.)
func (r *checkpointRestoreResult) reduceValue(key string, value any, reducer ReducerFunc) {
	if key == "" || value == nil {
		return
	}

	r.ensureStateOwned()

	current, exists := r.State[key]
	if !exists || current == nil {
		r.State[key] = value
		return
	}

	// Use the reducer to merge values
	r.State[key] = reducer.ReduceFn(current, value)
}

// restoreCheckpoint attempts to restore state from a checkpoint.
// Returns restored state data, pending writes, and any error that should abort execution.
// Note: State updates (WithStateUpdates) are applied later in initializeBSPState
// so they can use reducers for proper merging.
func restoreCheckpoint(
	ctx context.Context,
	checkpointCfg CheckpointConfig,
	runCfg *runConfig,
	yield func(message.Message, error) bool,
) (*checkpointRestoreResult, bool) {
	result := &checkpointRestoreResult{}

	// Step 1: Try to restore from checkpoint if autoRestore is enabled
	if !loadAutoRestoredCheckpoint(ctx, checkpointCfg, runCfg, result, yield) {
		return nil, false // Failed with error - abort execution
	}

	// Step 2: Apply explicit checkpoint if provided
	applyExplicitCheckpoint(runCfg, result)

	// Note: State updates from WithStateUpdates are applied in initializeBSPState
	// after BSP state is created, so they use reducers for proper merging.

	return result, true
}

// loadAutoRestoredCheckpoint tries to restore from checkpoint if autoRestore is enabled.
// Returns false if checkpoint loading failed and failOnCheckpointErr is true (abort execution).
func loadAutoRestoredCheckpoint(
	ctx context.Context,
	checkpointCfg CheckpointConfig,
	runCfg *runConfig,
	result *checkpointRestoreResult,
	yield func(message.Message, error) bool,
) bool {
	chkpt, err := tryAutoRestore(ctx, checkpointCfg, runCfg)
	if err != nil {
		if runCfg.failOnCheckpointErr {
			var zero message.Message
			yield(zero, fmt.Errorf("failed to load checkpoint: %w", err))
			return false
		}
		// Continue without checkpoint restoration
		return true
	}

	if chkpt != nil {
		result.useCheckpoint(chkpt)
	}

	return true
}

// applyExplicitCheckpoint applies a checkpoint that was explicitly provided via options.
func applyExplicitCheckpoint(runCfg *runConfig, result *checkpointRestoreResult) {
	if runCfg.checkpoint != nil {
		result.useCheckpoint(runCfg.checkpoint)
	}
}

// tryAutoRestore attempts to restore from checkpoint if autoRestore is enabled.
func tryAutoRestore(
	ctx context.Context,
	checkpointCfg CheckpointConfig,
	runCfg *runConfig,
) (*checkpoint.Checkpoint, error) {
	if !runCfg.autoRestore || checkpointCfg.Checkpointer == nil {
		return nil, nil
	}

	runID := runCfg.runID
	if runID == "" {
		runID = checkpointCfg.RunID
	}
	if runID == "" {
		return nil, nil
	}

	chkpt, err := checkpointCfg.Checkpointer.Load(ctx, runID)
	if err != nil {
		return nil, err
	}
	return chkpt, nil
}

func clonePendingWrites(writes []checkpoint.PendingWrite) []checkpoint.PendingWrite {
	if len(writes) == 0 {
		return nil
	}

	cloned := make([]checkpoint.PendingWrite, len(writes))
	copy(cloned, writes)
	return cloned
}

func cloneManagedValueDescriptors(desc []checkpoint.ManagedValueDescriptor) []checkpoint.ManagedValueDescriptor {
	if len(desc) == 0 {
		return nil
	}
	cloned := make([]checkpoint.ManagedValueDescriptor, len(desc))
	copy(cloned, desc)
	return cloned
}

func listManagedValueNames(desc []checkpoint.ManagedValueDescriptor, requiredOnly bool) []string {
	if len(desc) == 0 {
		return nil
	}
	names := make([]string, 0, len(desc))
	for _, d := range desc {
		if requiredOnly && !d.Required {
			continue
		}
		names = append(names, d.Name)
	}
	sort.Strings(names)
	return names
}

// applyInputToRestore applies input messages to the restored state.
func applyInputToRestore(
	cfg *ExecutorConfig,
	input []message.Message,
	restoreResult *checkpointRestoreResult,
) {
	keyName := MessagesKeyName
	// Only apply if the key has a registered reducer
	if reducer, ok := cfg.Execution.KeyRegistry[keyName]; ok {
		restoreResult.reduceValue(keyName, input, reducer)
	}
}

// PregelExecutor executes graphs using the Pregel BSP runtime.
type PregelExecutor struct {
	maxWorkers int
	maxSteps   int
	backend    DistributedBackend
}

// NewPregelExecutor creates a new Pregel executor with default settings.
// Uses DefaultMaxWorkers (4) and DefaultMaxSteps (100).
func NewPregelExecutor() *PregelExecutor {
	return &PregelExecutor{
		maxWorkers: DefaultMaxWorkers,
		maxSteps:   DefaultMaxSteps,
	}
}

// WithMaxWorkers sets the maximum number of parallel workers.
func (e *PregelExecutor) WithMaxWorkers(n int) *PregelExecutor {
	if n > 0 {
		e.maxWorkers = n
	}
	return e
}

// WithMaxSteps sets the maximum number of execution steps (iterations).
func (e *PregelExecutor) WithMaxSteps(maxSteps int) *PregelExecutor {
	if maxSteps > 0 {
		e.maxSteps = maxSteps
	}
	return e
}

// WithBackend sets a custom distributed backend for multi-node execution.
// The backend abstracts the underlying message-passing mechanism, allowing
// the graph to run across multiple machines without exposing Pregel concepts.
//
// Use NewPregelBackend() to adapt existing Pregel MessageBus implementations.
func (e *PregelExecutor) WithBackend(backend DistributedBackend) *PregelExecutor {
	e.backend = backend
	return e
}

// resultItem is a container for yielded outputs.
type resultItem struct {
	output message.Message
	err    error
}

const (
	// DefaultMaxWorkers is the default number of parallel workers for executing
	// graph nodes in the Pregel runtime. This controls concurrency of node execution
	// within each superstep.
	//
	// Why 4? Matches typical CPU core count for development machines. Production
	// deployments should tune this based on workload characteristics:
	//   - CPU-bound tasks: set to runtime.NumCPU()
	//   - I/O-bound tasks (API calls): can be 10-100x higher
	//   - Memory-constrained: reduce based on per-node memory footprint
	DefaultMaxWorkers = 4

	// DefaultMaxSteps is the default maximum number of supersteps before the
	// graph execution terminates. This prevents infinite loops in cyclic graphs
	// or runaway agent behavior.
	//
	// Why 100? Sufficient for most agent workflows:
	//   - Simple ReAct agents: 5-20 steps typical
	//   - Complex multi-agent: 20-50 steps typical
	//   - Research/exploration: may need 100+ (use WithMaxSteps to override)
	// If exceeded, execution stops with ErrMaxIterationsExceeded.
	DefaultMaxSteps = 100

	// DefaultCheckpointInterval is the default interval for saving checkpoints
	// during graph execution. Value of 1 means checkpoint after every superstep.
	//
	// Why 1? Ensures maximum recoverability at the cost of checkpoint overhead.
	// For long-running graphs with expensive checkpointing, increase via
	// graph.WithCheckpointInterval() to reduce I/O overhead.
	DefaultCheckpointInterval = 1

	// DefaultResultChanSize buffers outputs to prevent backpressure when the yield
	// consumer is slower than the producer. This provides smoother execution flow
	// without blocking nodes. Typical agents produce <10 results/superstep.
	//
	// Why 100? Sized for ~10 supersteps worth of buffering (10 results/step * 10 steps).
	// Large enough to prevent blocking during brief consumer slowdowns, small enough
	// to avoid excessive memory usage (~8KB for typical output types).
	DefaultResultChanSize = 100
)

// startResultConsumer starts a goroutine that consumes results from the channel
// and yields them sequentially. Returns a done channel that closes when the consumer exits.
func startResultConsumer(ctx context.Context, cancel context.CancelFunc, resultChan <-chan resultItem, yield func(message.Message, error) bool) <-chan struct{} {
	yieldDone := make(chan struct{})
	go func() {
		defer close(yieldDone)
		for {
			select {
			case <-ctx.Done():
				chanutil.DrainUntilClosed(resultChan)
				return
			case item, ok := <-resultChan:
				if !ok {
					return
				}
				if !yield(item.output, item.err) {
					cancel()
					chanutil.DrainUntilClosed(resultChan)
					return
				}
			}
		}
	}()
	return yieldDone
}

// buildRuntimeOptions constructs the Pregel runtime options with event callbacks.
func buildRuntimeOptions(
	e *PregelExecutor,
	runCfg *runConfig,
	adapter *pregelGraphAdapter,
) []pregel.RuntimeOption[*ExecutorConfig, Updates] {
	maxWorkers := e.maxWorkers
	if runCfg.maxConcurrency > 0 {
		maxWorkers = runCfg.maxConcurrency
	}

	maxSteps := e.maxSteps
	if runCfg.maxIterations > 0 {
		maxSteps = runCfg.maxIterations
	}

	opts := []pregel.RuntimeOption[*ExecutorConfig, Updates]{
		pregel.WithMaxWorkers[*ExecutorConfig, Updates](maxWorkers),
		pregel.WithMaxIterations[*ExecutorConfig, Updates](maxSteps),
		pregel.WithOnSuperstepStart[*ExecutorConfig, Updates](
			func(ctx context.Context, superstep int64, frontier pregel.FrontierInfo) error {
				// Track current superstep for interrupt checkpoints
				adapter.superstep = int(superstep)
				event.Publish(ctx, event.Event{
					Type:      event.EventSuperstepStart,
					Superstep: int(superstep),
					Timestamp: time.Now(),
					Data: map[string]any{
						"frontier_size":  frontier.Size,
						"frontier_nodes": frontier.Nodes,
					},
				})
				return nil
			},
		),
		pregel.WithOnSuperstepComplete[*ExecutorConfig, Updates](
			func(ctx context.Context, superstep int64) error {
				if err := adapter.twoPhaseCommit(ctx, superstep); err != nil {
					return err
				}
				event.Publish(ctx, event.Event{
					Type:      event.EventSuperstepComplete,
					Superstep: int(superstep),
					Timestamp: time.Now(),
					Data:      map[string]any{},
				})
				return nil
			},
		),
	}

	// Convert graph-layer backend to Pregel MessageBus internally
	if e.backend != nil {
		bus := &backendToMessageBusAdapter{backend: e.backend}
		opts = append(opts, pregel.WithMessageBus[*ExecutorConfig](bus))
	}

	return opts
}

// publishCompletionEvent publishes the graph completion or error event.
func publishCompletionEvent(ctx context.Context, runID string, runtimeErr error) {
	if runtimeErr == nil {
		event.Publish(ctx, event.Event{
			Type:      event.EventGraphComplete,
			Timestamp: time.Now(),
			Data:      map[string]any{"run_id": runID},
		})
	} else {
		event.Publish(ctx, event.Event{
			Type:      event.EventGraphError,
			Timestamp: time.Now(),
			Error:     runtimeErr.Error(),
			Data:      map[string]any{"run_id": runID},
		})
	}
}

// initializeBSPState creates and configures the BSP state manager from restore result.
// Handles managed values rehydration, state validation, pending writes recovery,
// and state updates from WithStateUpdates (applied via reducers for proper merging).
func initializeBSPState(
	ctx context.Context,
	restoreResult *checkpointRestoreResult,
	runCfg *runConfig,
	keyRegistry KeyRegistry,
) (*BSPState, error) {
	// Validate and rehydrate managed values from checkpoint
	if err := rehydrateManagedValues(ctx, restoreResult.ManagedValues, runCfg); err != nil {
		return nil, err
	}

	// Security: Validate checkpoint state contains only declared keys
	// This prevents state injection attacks via corrupted/malicious checkpoints
	validatedState, err := validateCheckpointState(restoreResult.State, keyRegistry)
	if err != nil {
		return nil, fmt.Errorf("invalid checkpoint state: %w", err)
	}

	// Create BSP-compliant state manager with reducer registry
	bspState := NewBSPState(validatedState, keyRegistry)

	// Attach managed values to BSP state (accessible via View)
	if runCfg.managedValues != nil {
		bspState.setManagedValues(runCfg.managedValues)
	}

	// Two-phase commit recovery: apply pending writes from uncommitted checkpoint
	// Also validate pending writes against key registry
	if len(restoreResult.PendingWrites) > 0 {
		validatedWrites, err := validatePendingWrites(restoreResult.PendingWrites, keyRegistry)
		if err != nil {
			return nil, fmt.Errorf("invalid checkpoint pending writes: %w", err)
		}
		bspState.ApplyPendingWrites(validatedWrites)
	}

	// Apply state updates from WithStateUpdates using reducers for proper merging
	// This enables human-in-the-loop workflows where new input is merged with checkpoint state
	if len(runCfg.stateUpdates) > 0 {
		bspState.Write("__resume__", Updates(runCfg.stateUpdates))
		bspState.CommitBarrier() // Make updates visible immediately
	}

	return bspState, nil
}

// CheckpointStateError indicates that a checkpoint contains invalid state.
type CheckpointStateError struct {
	UnknownKeys []string // Keys in checkpoint that are not registered in the graph
}

func (e *CheckpointStateError) Error() string {
	return fmt.Sprintf("checkpoint contains unknown state keys: %v", e.UnknownKeys)
}

// validateCheckpointState validates that all keys in the checkpoint state are
// registered in the KeyRegistry. Returns an error if unknown keys are found.
// This prevents state injection attacks via corrupted/malicious checkpoints.
func validateCheckpointState(state map[string]any, keyRegistry KeyRegistry) (map[string]any, error) {
	if state == nil {
		return nil, nil
	}

	// If no keys are registered, any state is invalid
	if len(keyRegistry) == 0 && len(state) > 0 {
		unknownKeys := make([]string, 0, len(state))
		for key := range state {
			unknownKeys = append(unknownKeys, key)
		}
		return nil, &CheckpointStateError{UnknownKeys: unknownKeys}
	}

	var unknownKeys []string
	validated := make(map[string]any, len(state))

	for key, value := range state {
		if _, ok := keyRegistry[key]; ok {
			validated[key] = value
		} else {
			unknownKeys = append(unknownKeys, key)
		}
	}

	if len(unknownKeys) > 0 {
		return nil, &CheckpointStateError{UnknownKeys: unknownKeys}
	}

	if len(validated) == 0 {
		return nil, nil
	}
	return validated, nil
}

// applyApprovalsAndCheckpoint applies approval state updates and saves a new checkpoint.
// This implements the approval flow: apply approvals to state → save checkpoint → resume
func applyApprovalsAndCheckpoint(
	ctx context.Context,
	bspState *BSPState,
	runCfg *runConfig,
	checkpointCfg CheckpointConfig,
) error {
	if len(runCfg.approvals) == 0 {
		return nil
	}

	// Get existing approvals from state or create new map
	var approvals map[string]*ApprovalResponse
	if existing, ok := bspState.ReadView().GetValue(ApprovalsKey); ok {
		if existingMap, ok := existing.(map[string]*ApprovalResponse); ok {
			approvals = existingMap
		}
	}
	if approvals == nil {
		approvals = make(map[string]*ApprovalResponse)
	}

	// Merge new approvals
	for nodeName, approval := range runCfg.approvals {
		approvals[nodeName] = approval
	}

	// Write approvals to state (using system node name for provenance)
	bspState.Write("__system__", Updates{ApprovalsKey: approvals})
	bspState.CommitBarrier()

	// Save new checkpoint with approvals applied
	// Keep pausedNodes - they indicate where to resume from
	if err := saveApprovalCheckpoint(ctx, bspState, runCfg, checkpointCfg); err != nil {
		return err
	}

	// DON'T clear pausedNodes - RootVertices needs them to know where to start

	return nil
}

// saveApprovalCheckpoint saves a checkpoint with approval state applied.
func saveApprovalCheckpoint(
	ctx context.Context,
	bspState *BSPState,
	runCfg *runConfig,
	checkpointCfg CheckpointConfig,
) error {
	if checkpointCfg.Checkpointer == nil {
		return nil
	}

	runID := runCfg.runID
	if runID == "" {
		runID = checkpointCfg.RunID
	}

	if runID == "" {
		return nil
	}

	cp := &checkpoint.Checkpoint{
		RunID:       runID,
		State:       bspState.Snapshot(),
		PausedNodes: runCfg.pausedNodes,
		Committed:   true,
		Timestamp:   time.Now(),
	}

	if err := checkpointCfg.Checkpointer.Save(ctx, cp); err != nil {
		if runCfg.failOnCheckpointErr {
			return fmt.Errorf("failed to save approval checkpoint: %w", err)
		}
	}

	return nil
}

// validatePendingWrites validates that all pending writes reference keys
// registered in the KeyRegistry. Returns an error if unknown keys are found.
func validatePendingWrites(writes []checkpoint.PendingWrite, keyRegistry KeyRegistry) ([]checkpoint.PendingWrite, error) {
	if len(writes) == 0 {
		return nil, nil
	}

	var unknownKeys []string
	validated := make([]checkpoint.PendingWrite, 0, len(writes))

	for _, write := range writes {
		if _, ok := keyRegistry[write.Channel]; ok {
			validated = append(validated, write)
		} else {
			unknownKeys = append(unknownKeys, write.Channel)
		}
	}

	if len(unknownKeys) > 0 {
		return nil, &CheckpointStateError{UnknownKeys: unknownKeys}
	}

	return validated, nil
}

// rehydrateManagedValues validates checkpoint managed values and rehydrates them into runCfg.
func rehydrateManagedValues(ctx context.Context, managedValues []checkpoint.ManagedValueDescriptor, runCfg *runConfig) error {
	if len(managedValues) == 0 {
		return nil
	}

	if runCfg.managedValues == nil {
		required := listManagedValueNames(managedValues, true)
		if len(required) > 0 {
			return &ManagedValueError{MissingValues: required, IsRequired: false}
		}
		return nil
	}

	return runCfg.managedValues.ensureAndRehydrate(ctx, managedValues)
}

// executionContext holds runtime context for graph execution.
type executionContext struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cfg        *ExecutorConfig
	runCfg     *runConfig
	resultChan chan resultItem
	yieldDone  <-chan struct{}
}

// createPregelAdapter creates the graph adapter for the Pregel runtime.
func createPregelAdapter(
	execCtx *executionContext,
	bspState *BSPState,
) *pregelGraphAdapter {
	// Thread-safe yield function that sends to channel instead of calling yield directly
	safeYield := func(output message.Message, err error) bool {
		select {
		case execCtx.resultChan <- resultItem{output: output, err: err}:
			return true
		case <-execCtx.ctx.Done():
			return false
		}
	}

	return &pregelGraphAdapter{
		cfg:                execCtx.cfg,
		runCfg:             execCtx.runCfg,
		bspState:           bspState,
		safeYield:          safeYield,
		nodeMiddleware:     execCtx.cfg.Execution.NodeMiddleware,
		superstep:          0,
		checkpointInterval: execCtx.runCfg.checkpointInterval,
	}
}

// runPregelRuntime creates and executes the Pregel runtime.
func runPregelRuntime(
	execCtx *executionContext,
	e *PregelExecutor,
	adapter *pregelGraphAdapter,
) error {
	// Build runtime options with event publishing callbacks
	runtimeOpts := buildRuntimeOptions(e, execCtx.runCfg, adapter)

	// Publish graph start event
	event.Publish(execCtx.ctx, event.Event{
		Type:      event.EventGraphStart,
		Timestamp: time.Now(),
		Data: map[string]any{
			"run_id":       execCtx.runCfg.runID,
			"entry_points": execCtx.cfg.Execution.EntryPoints,
		},
	})

	// Create the Pregel runtime
	rt, err := pregel.NewRuntime(adapter, runtimeOpts...)
	if err != nil {
		return err
	}

	// Track runtime error for completion event
	var runtimeErr error

	// Run the runtime - it will call adapter methods
	for _, err := range rt.Run(execCtx.ctx) {
		if err != nil {
			runtimeErr = err
			select {
			case execCtx.resultChan <- resultItem{err: err}:
			case <-execCtx.ctx.Done():
			}
		}
	}

	return runtimeErr
}

// Run executes the graph using the Pregel BSP runtime.
func (e *PregelExecutor) Run(ctx context.Context, cfg *ExecutorConfig, input []message.Message, opts ...runOption) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Apply run options
		runCfg := &runConfig{
			checkpointInterval: DefaultCheckpointInterval,
		}
		for _, opt := range opts {
			opt(runCfg)
		}

		// Restore state from checkpoint if configured
		restoreResult, ok := restoreCheckpoint(ctx, cfg.Checkpoint, runCfg, yield)
		if !ok {
			return
		}

		// Capture paused nodes from checkpoint for resume
		if len(restoreResult.PausedNodes) > 0 {
			runCfg.pausedNodes = restoreResult.PausedNodes
		}

		// Only merge input if not in resume mode (skipInputMerge = false)
		// Resume() sets skipInputMerge to prevent zero-value input from overwriting state
		if !runCfg.skipInputMerge {
			applyInputToRestore(cfg, input, restoreResult)
		}

		// Initialize BSP state with managed values, pending writes, and key registry
		bspState, err := initializeBSPState(ctx, restoreResult, runCfg, cfg.Execution.KeyRegistry)
		if err != nil {
			var zero message.Message
			yield(zero, err)
			return
		}

		// Apply approvals to state and save new checkpoint
		// This happens BEFORE running, so the interrupt check sees the approvals in state
		if err := applyApprovalsAndCheckpoint(ctx, bspState, runCfg, cfg.Checkpoint); err != nil {
			var zero message.Message
			yield(zero, err)
			return
		}

		resultChan := make(chan resultItem, DefaultResultChanSize)
		yieldDone := startResultConsumer(ctx, cancel, resultChan, yield)

		// Build execution context
		execCtx := &executionContext{
			ctx:        ctx,
			cancel:     cancel,
			cfg:        cfg,
			runCfg:     runCfg,
			resultChan: resultChan,
			yieldDone:  yieldDone,
		}

		// Create the Pregel adapter and run the runtime
		adapter := createPregelAdapter(execCtx, bspState)
		runtimeErr := runPregelRuntime(execCtx, e, adapter)

		// Close result channel and wait for consumer to finish
		close(resultChan)
		<-yieldDone

		// Publish graph completion event
		publishCompletionEvent(ctx, runCfg.runID, runtimeErr)
	}
}

// pregelGraphAdapter adapts ExecutorConfig to the pregel.Graph interface.
type pregelGraphAdapter struct {
	cfg            *ExecutorConfig
	runCfg         *runConfig
	bspState       *BSPState                         // BSP-compliant state with read snapshots and write buffering
	safeYield      func(message.Message, error) bool // Thread-safe yield via channel
	nodeMiddleware []NodeMiddleware

	superstep          int
	checkpointInterval int
}

// RootVertices returns the starting vertices for execution.
// When resuming from an interrupt, returns the paused nodes instead of entry points.
func (a *pregelGraphAdapter) RootVertices() []string {
	if len(a.runCfg.pausedNodes) > 0 {
		return a.runCfg.pausedNodes
	}
	return a.cfg.Execution.EntryPoints
}

// Outgoing returns the target nodes for a given node.
func (a *pregelGraphAdapter) Outgoing(vertex string) []string {
	if node, ok := a.cfg.Execution.Nodes[vertex]; ok {
		return node.Targets
	}
	return nil
}

// VertexByName returns a vertex adapter for the given node.
func (a *pregelGraphAdapter) VertexByName(name string) pregel.Vertex[*ExecutorConfig, Updates] {
	return &pregelVertexAdapter{
		name:    name,
		adapter: a,
	}
}

// State returns the executor configuration.
func (a *pregelGraphAdapter) State() *ExecutorConfig {
	return a.cfg
}

func (a *pregelGraphAdapter) managedValueDescriptors() []checkpoint.ManagedValueDescriptor {
	if a.runCfg.managedValues == nil {
		return nil
	}
	return a.runCfg.managedValues.descriptors()
}

// yieldMessages yields each message from a slice of messages.
// Lock-free: uses buffered channel for thread-safe parallel node execution.
func (a *pregelGraphAdapter) yieldMessages(messages []message.Message) {
	for _, msg := range messages {
		if !a.safeYield(msg, nil) {
			return
		}
	}
}

// yieldUpdates yields messages from updates if the messages key is present.
func (a *pregelGraphAdapter) yieldUpdates(updates Updates) {
	val, ok := updates[MessagesKeyName]
	if !ok {
		return
	}

	if msgs, ok := val.([]message.Message); ok {
		a.yieldMessages(msgs)
	}
}

// checkInterrupt checks if an interrupt is needed and returns an error if so.
// When an interrupt occurs, a checkpoint is saved with the paused node information.
func (a *pregelGraphAdapter) checkInterrupt(
	ctx context.Context,
	nodeName string,
	icfg *interruptConfig,
	isBefore bool,
) error {
	needsApproval := true
	if icfg.guard != nil {
		// Use BSP read view for guard evaluation
		view := a.bspState.ReadView()
		var err error
		needsApproval, _, err = icfg.guard(ctx, view)
		if err != nil {
			return err
		}
	}
	if needsApproval {
		// Check approvals from state (approvals are persisted as state updates)
		approvals := a.getApprovalsFromState()
		if approvals == nil || approvals[nodeName] == nil {
			// Save interrupt checkpoint before returning error
			a.saveInterruptCheckpoint(ctx, nodeName)
			return &InterruptError{NodeName: nodeName, Before: isBefore}
		}
	}
	return nil
}

// saveInterruptCheckpoint saves a checkpoint when an interrupt occurs.
// This captures the current state and marks which node is paused.
func (a *pregelGraphAdapter) saveInterruptCheckpoint(ctx context.Context, pausedNode string) {
	checkpointerEnabled := a.cfg.Checkpoint.Checkpointer != nil
	runID := a.runCfg.runID
	if runID == "" {
		runID = a.cfg.Checkpoint.RunID
	}
	if !checkpointerEnabled || runID == "" {
		return
	}

	// Commit any pending writes before saving checkpoint
	a.bspState.CommitBarrier()

	cp := &checkpoint.Checkpoint{
		RunID:         runID,
		Superstep:     int64(a.superstep),
		State:         a.bspState.Snapshot(),
		PausedNodes:   []string{pausedNode},
		Committed:     true,
		Timestamp:     time.Now(),
		ManagedValues: a.managedValueDescriptors(),
	}

	// Save checkpoint - ignore errors since we're about to return an interrupt error anyway
	_ = a.cfg.Checkpoint.Checkpointer.Save(ctx, cp)
}

// getApprovalsFromState retrieves the approvals map from BSP state.
func (a *pregelGraphAdapter) getApprovalsFromState() map[string]*ApprovalResponse {
	view := a.bspState.ReadView()
	if approvals, ok := view.GetValue(ApprovalsKey); ok {
		if approvalsMap, ok := approvals.(map[string]*ApprovalResponse); ok {
			return approvalsMap
		}
	}
	return nil
}

// pregelVertexAdapter adapts a node to the pregel.Vertex interface.
type pregelVertexAdapter struct {
	name    string
	adapter *pregelGraphAdapter
}

// Name returns the vertex name.
func (v *pregelVertexAdapter) Name() string {
	return v.name
}

// Run executes the vertex computation.
func (v *pregelVertexAdapter) Run(
	ctx context.Context,
	vctx pregel.VertexContext[*ExecutorConfig, Updates],
	incoming []pregel.Message[Updates],
) error {
	node, ok := v.adapter.cfg.Execution.Nodes[v.name]
	if !ok {
		return nil
	}

	// Publish node start event
	event.Publish(ctx, event.Event{
		Type:      event.EventNodeStart,
		Node:      v.name,
		Timestamp: time.Now(),
		Data:      map[string]any{},
	})

	// Track node execution timing
	nodeStart := time.Now()
	var nodeErr error
	defer func() {
		duration := time.Since(nodeStart)
		if nodeErr != nil {
			event.Publish(ctx, event.Event{
				Type:      event.EventNodeError,
				Node:      v.name,
				Timestamp: time.Now(),
				Duration:  duration,
				Error:     nodeErr.Error(),
				Data:      map[string]any{},
			})
		} else {
			event.Publish(ctx, event.Event{
				Type:      event.EventNodeComplete,
				Node:      v.name,
				Timestamp: time.Now(),
				Duration:  duration,
				Data:      map[string]any{},
			})
		}
	}()

	// Note: BSP semantics - all nodes in this superstep read from the same
	// snapshot (from previous superstep). Writes are buffered and only
	// become visible after the superstep barrier commits.

	// Check for interrupt before
	if icfg, hasInterrupt := v.adapter.cfg.Interrupt.Before[v.name]; hasInterrupt {
		if err := v.adapter.checkInterrupt(ctx, v.name, icfg, true); err != nil {
			nodeErr = err
			return err
		}
	}

	// Create BSP read view for node execution (reads from previous superstep snapshot)
	// Wrap with node context so NodeName() is available
	roScope := withNodeName(v.adapter.bspState.ReadView(), v.name)

	// Create scope with typed streaming capability
	// Stream function yields partial values directly to output (e.g., LLM chunks, tool progress)
	streamFn := func(value message.Message) {
		// Publish node stream event for observability
		event.Publish(ctx, event.Event{
			Type:      event.EventNodeStream,
			Node:      v.name,
			Timestamp: time.Now(),
			Data:      map[string]any{},
		})
		// Yield directly to output stream (no state update - BSP handles state)
		v.adapter.safeYield(value, nil)
	}
	scope := newScope(roScope, streamFn)

	// Apply node middleware
	fn := node.Fn
	for i := len(v.adapter.nodeMiddleware) - 1; i >= 0; i-- {
		fn = v.adapter.nodeMiddleware[i](fn)
	}

	// Execute node
	cmd, err := fn(ctx, scope)
	if err != nil {
		nodeErr = err
		return err
	}

	// Buffer updates for commit at superstep barrier (BSP semantics)
	// Writes are not visible to other nodes until after barrier
	v.adapter.bspState.Write(v.name, cmd.Updates)

	// Yield outputs immediately (for streaming)
	v.adapter.yieldUpdates(cmd.Updates)

	// Check for interrupt after
	if icfg, hasInterrupt := v.adapter.cfg.Interrupt.After[v.name]; hasInterrupt {
		if err := v.adapter.checkInterrupt(ctx, v.name, icfg, false); err != nil {
			nodeErr = err
			return err
		}
	}

	// Send messages to next nodes
	for _, next := range cmd.Next {
		if next == END {
			continue
		}
		// Convert graph.Updates to Updates
		stateUpdates := make(Updates, len(cmd.Updates))
		maps.Copy(stateUpdates, cmd.Updates)
		vctx.Send(pregel.Message[Updates]{
			From: v.name,
			To:   next,
			Data: stateUpdates,
		})
	}

	return nil
}

// twoPhaseCommit implements the two-phase commit protocol for checkpointing.
// Phase 1: Save checkpoint with pending writes (Committed=false) - captures writes before barrier
// Phase 2: Commit BSP barrier, save checkpoint (Committed=true) - marks transaction complete
// This ensures crash recovery can either:
//   - Re-apply pending writes if crash occurred before barrier (Committed=false)
//   - Skip writes if crash occurred after barrier (Committed=true)
func (a *pregelGraphAdapter) twoPhaseCommit(ctx context.Context, superstep int64) error {
	// Check if checkpointing is configured
	checkpointerEnabled := a.cfg.Checkpoint.Checkpointer != nil
	runID := a.runCfg.runID
	if runID == "" {
		runID = a.cfg.Checkpoint.RunID
	}
	checkpointerEnabled = checkpointerEnabled && runID != ""

	// Check if we should save based on interval
	shouldCheckpoint := checkpointerEnabled &&
		(a.checkpointInterval <= 0 || int(superstep)%a.checkpointInterval == 0)

	// Load current PausedNodes to preserve interrupt state across two-phase commit
	// This is critical: interrupt checkpoints set PausedNodes, and we must not lose them
	var currentPausedNodes []string
	if shouldCheckpoint {
		if existingCp, err := a.cfg.Checkpoint.Checkpointer.Load(ctx, runID); err == nil && existingCp != nil {
			currentPausedNodes = existingCp.PausedNodes
		}
	}

	// Phase 1: Save checkpoint with pending writes (before barrier commit)
	// This captures the state BEFORE writes are applied, along with the pending writes
	// If a crash occurs here, we can re-apply the pending writes on recovery
	if shouldCheckpoint && a.bspState.HasPendingWrites() {
		pendingWrites := a.bspState.PendingWrites()
		phase1Checkpoint := &checkpoint.Checkpoint{
			RunID:         runID,
			Superstep:     superstep,
			State:         a.bspState.Snapshot(), // Committed state BEFORE barrier
			PendingWrites: pendingWrites,
			PausedNodes:   currentPausedNodes, // Preserve interrupt state
			Committed:     false,              // Mark as uncommitted - pending writes not yet applied
			Timestamp:     time.Now(),
			ManagedValues: a.managedValueDescriptors(),
		}

		if err := a.cfg.Checkpoint.Checkpointer.Save(ctx, phase1Checkpoint); err != nil {
			if a.runCfg.failOnCheckpointErr {
				return fmt.Errorf("two-phase commit phase 1 failed at superstep %d: %w", superstep, err)
			}
			// Continue without checkpoint
		}
	}

	// BSP Barrier: Commit all buffered writes and create new read snapshot
	// After this, all writes from this superstep become visible to the next
	a.bspState.CommitBarrier()

	// Take a single snapshot of the committed state (used for both event and checkpoint)
	committedState := a.bspState.Snapshot()

	// Publish state update event after barrier commit with the new state
	event.Publish(ctx, event.Event{
		Type:      event.EventStateUpdate,
		Superstep: int(superstep),
		Timestamp: time.Now(),
		Data:      committedState,
	})

	// Phase 2: Save checkpoint with Committed=true (after barrier commit)
	// This marks the transaction as complete - all pending writes have been applied
	// If a crash occurs after this, we skip re-applying writes on recovery
	if shouldCheckpoint {
		phase2Checkpoint := &checkpoint.Checkpoint{
			RunID:         runID,
			Superstep:     superstep,
			State:         committedState,     // Committed state AFTER barrier
			PendingWrites: nil,                // No pending writes - all committed
			PausedNodes:   currentPausedNodes, // Preserve interrupt state
			Committed:     true,               // Mark as committed
			Timestamp:     time.Now(),
			ManagedValues: a.managedValueDescriptors(),
		}

		if err := a.cfg.Checkpoint.Checkpointer.Save(ctx, phase2Checkpoint); err != nil {
			if a.runCfg.failOnCheckpointErr {
				return fmt.Errorf("two-phase commit phase 2 failed at superstep %d: %w", superstep, err)
			}
			// Continue - Phase 1 checkpoint can be used for recovery
		}
	}

	return nil
}
