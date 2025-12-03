package graph

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/internal/chanutil"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/pregel"
)

// isNilOrZero checks if a value is nil, zero, or empty for its type.
// Used to determine if an input should overwrite restored checkpoint state.
// For slices and maps, empty (len=0) is treated as "no input provided".
func isNilOrZero[T any](v T) bool {
	val := reflect.ValueOf(v)
	// Check if interface is nil
	if !val.IsValid() {
		return true
	}
	// Check if value is nil or empty (for slices, maps, channels)
	switch val.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Func, reflect.Interface:
		return val.IsNil()
	case reflect.Slice, reflect.Map:
		// Treat empty slices/maps as "no input" to preserve checkpoint state
		return val.IsNil() || val.Len() == 0
	}
	// Check if value equals zero value
	return val.IsZero()
}

// checkpointRestoreResult contains restored state and pending writes from checkpoint.
type checkpointRestoreResult struct {
	State         map[string]any
	stateOwned    bool
	PendingWrites []checkpoint.PendingWrite // Only set if Committed=false
	ManagedValues []checkpoint.ManagedValueDescriptor
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

func (r *checkpointRestoreResult) applyUpdates(updates map[string]any) {
	if len(updates) == 0 {
		return
	}
	r.ensureStateOwned()
	maps.Copy(r.State, updates)
}

func (r *checkpointRestoreResult) setValue(key string, value any) {
	if key == "" {
		return
	}
	r.ensureStateOwned()
	r.State[key] = value
}

// restoreCheckpoint attempts to restore state from a checkpoint.
// Returns restored state data, pending writes, and any error that should abort execution.
func restoreCheckpoint[O any](
	ctx context.Context,
	cfg *ExecutorConfig[any, O],
	runCfg *runConfig,
	yield func(O, error) bool,
) (*checkpointRestoreResult, bool) {
	result := &checkpointRestoreResult{}

	// Try to restore from checkpoint if autoRestore is enabled
	if chkpt, err := tryAutoRestore(ctx, cfg.Checkpointer, cfg.RunID, runCfg); err != nil {
		if runCfg.failOnCheckpointErr {
			var zero O
			yield(zero, fmt.Errorf("failed to load checkpoint: %w", err))
			return nil, false
		}
		// Continue without checkpoint restoration
	} else if chkpt != nil {
		result.useCheckpoint(chkpt)
	}

	// If a checkpoint is explicitly provided, use it
	if runCfg.checkpoint != nil {
		result.useCheckpoint(runCfg.checkpoint)
	}

	// Apply any state updates provided via WithStateUpdates
	// This enables human-in-the-loop workflows to inject human input
	result.applyUpdates(runCfg.stateUpdates)

	return result, true
}

// tryAutoRestore attempts to restore from checkpoint if autoRestore is enabled.
func tryAutoRestore(
	ctx context.Context,
	checkpointer checkpoint.Checkpointer,
	cfgRunID string,
	runCfg *runConfig,
) (*checkpoint.Checkpoint, error) {
	if !runCfg.autoRestore || checkpointer == nil {
		return nil, nil
	}

	runID := runCfg.runID
	if runID == "" {
		runID = cfgRunID
	}
	if runID == "" {
		return nil, nil
	}

	chkpt, err := checkpointer.Load(ctx, runID)
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

// PregelExecutor executes graphs using the Pregel BSP runtime.
type PregelExecutor[I, O any] struct {
	maxWorkers int
	maxSteps   int
	backend    DistributedBackend
}

// NewPregelExecutor creates a new Pregel executor with default settings.
func NewPregelExecutor[I, O any]() *PregelExecutor[I, O] {
	return &PregelExecutor[I, O]{
		maxWorkers: 4,
		maxSteps:   100,
	}
}

// WithMaxWorkers sets the maximum number of parallel workers.
func (e *PregelExecutor[I, O]) WithMaxWorkers(n int) *PregelExecutor[I, O] {
	if n > 0 {
		e.maxWorkers = n
	}
	return e
}

// WithMaxSteps sets the maximum number of execution steps (iterations).
func (e *PregelExecutor[I, O]) WithMaxSteps(maxSteps int) *PregelExecutor[I, O] {
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
func (e *PregelExecutor[I, O]) WithBackend(backend DistributedBackend) *PregelExecutor[I, O] {
	e.backend = backend
	return e
}

// resultItem is a container for yielded outputs.
type resultItem[O any] struct {
	output O
	err    error
}

// defaultResultChanSize buffers outputs to prevent backpressure when the yield
// consumer is slower than the producer. This provides smoother execution flow
// without blocking nodes. Typical agents produce <10 results/superstep.
//
// Why 100? Sized for ~10 supersteps worth of buffering (10 results/step * 10 steps).
// Large enough to prevent blocking during brief consumer slowdowns, small enough
// to avoid excessive memory usage (~8KB for typical output types).
const defaultResultChanSize = 100

// startResultConsumer starts a goroutine that consumes results from the channel
// and yields them sequentially. Returns a done channel that closes when the consumer exits.
func startResultConsumer[O any](ctx context.Context, cancel context.CancelFunc, resultChan <-chan resultItem[O], yield func(O, error) bool) <-chan struct{} {
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
func buildRuntimeOptions[I, O any](
	e *PregelExecutor[I, O],
	runCfg *runConfig,
	adapter *pregelGraphAdapter[I, O],
) []pregel.RuntimeOption[*ExecutorConfig[I, O], Updates] {
	maxWorkers := e.maxWorkers
	if runCfg.maxConcurrency > 0 {
		maxWorkers = runCfg.maxConcurrency
	}

	maxSteps := e.maxSteps
	if runCfg.maxIterations > 0 {
		maxSteps = runCfg.maxIterations
	}

	opts := []pregel.RuntimeOption[*ExecutorConfig[I, O], Updates]{
		pregel.WithMaxWorkers[*ExecutorConfig[I, O], Updates](maxWorkers),
		pregel.WithMaxIterations[*ExecutorConfig[I, O], Updates](maxSteps),
		pregel.WithOnSuperstepStart[*ExecutorConfig[I, O], Updates](
			func(ctx context.Context, superstep int64, frontier pregel.FrontierInfo) error {
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
		pregel.WithOnSuperstepComplete[*ExecutorConfig[I, O], Updates](
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
		opts = append(opts, pregel.WithMessageBus[*ExecutorConfig[I, O]](bus))
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

// Run executes the graph using the Pregel BSP runtime.
//
//nolint:gocyclo,nestif // Coordination of checkpoint restore, BSP plumbing, and streaming yield is centralized here.
func (e *PregelExecutor[I, O]) Run(ctx context.Context, cfg *ExecutorConfig[I, O], input I, opts ...RunOption) iter.Seq2[O, error] {
	return func(yield func(O, error) bool) {
		// Create cancellable context for early termination
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Apply run options
		runCfg := &runConfig{
			checkpointInterval: 1, // Default: save every superstep
		}
		for _, opt := range opts {
			opt(runCfg)
		}

		// Restore state from checkpoint if configured
		adaptedCfg := &ExecutorConfig[any, O]{
			Checkpointer: cfg.Checkpointer,
			RunID:        cfg.RunID,
		}
		restoreResult, ok := restoreCheckpoint(ctx, adaptedCfg, runCfg, yield)
		if !ok {
			return
		}

		// Set the input into state under the output key if:
		// 1. An output key is defined, AND
		// 2. The input is not nil (to avoid overwriting restored checkpoint state with nil)
		// This allows checkpoint restoration to preserve state while still accepting new input
		if cfg.OutputKey != "" && !isNilOrZero(input) {
			restoreResult.setValue(cfg.OutputKey, input)
		}

		// Create BSP-compliant state manager
		bspState := NewBSPState(restoreResult.State)

		if len(restoreResult.ManagedValues) > 0 {
			if runCfg.managedValues == nil {
				required := listManagedValueNames(restoreResult.ManagedValues, true)
				if len(required) > 0 {
					var zero O
					err := fmt.Errorf("graph: checkpoint requires managed values (%s); provide them via graph.WithManagedValues when resuming", strings.Join(required, ", "))
					yield(zero, err)
					return
				}
			} else if err := runCfg.managedValues.ensureAndRehydrate(ctx, restoreResult.ManagedValues); err != nil {
				var zero O
				yield(zero, err)
				return
			}
		}

		// Attach managed values to BSP state (accessible via View)
		if runCfg.managedValues != nil {
			bspState.setManagedValues(runCfg.managedValues)
		}

		// Two-phase commit recovery: apply pending writes from uncommitted checkpoint
		if len(restoreResult.PendingWrites) > 0 {
			bspState.ApplyPendingWrites(restoreResult.PendingWrites)
		}

		// Create buffered result channel for lock-free output collection
		resultChan := make(chan resultItem[O], defaultResultChanSize)

		// Start consumer goroutine that yields results sequentially
		yieldDone := startResultConsumer(ctx, cancel, resultChan, yield)

		// Thread-safe yield function that sends to channel instead of calling yield directly
		safeYield := func(output O, err error) bool {
			select {
			case resultChan <- resultItem[O]{output: output, err: err}:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// Create the graph adapter for the Pregel runtime
		adapter := &pregelGraphAdapter[I, O]{
			cfg:                cfg,
			runCfg:             runCfg,
			bspState:           bspState,
			safeYield:          safeYield,
			middleware:         cfg.Middleware,
			superstep:          0,
			checkpointInterval: runCfg.checkpointInterval,
		}

		// Build runtime options with event publishing callbacks
		runtimeOpts := buildRuntimeOptions(e, runCfg, adapter)

		// Publish graph start event
		event.Publish(ctx, event.Event{
			Type:      event.EventGraphStart,
			Timestamp: time.Now(),
			Data: map[string]any{
				"run_id":       runCfg.runID,
				"entry_points": cfg.EntryPoints,
			},
		})

		// Create and run the Pregel runtime
		rt, err := pregel.NewRuntime(adapter, runtimeOpts...)
		if err != nil {
			event.Publish(ctx, event.Event{
				Type:      event.EventGraphError,
				Timestamp: time.Now(),
				Error:     err.Error(),
				Data:      map[string]any{"run_id": runCfg.runID},
			})
			close(resultChan)
			<-yieldDone
			var zero O
			yield(zero, err)
			return
		}

		// Track runtime error for completion event
		var runtimeErr error

		// Run the runtime - it will call adapter methods
		for _, err := range rt.Run(ctx) {
			if err != nil {
				runtimeErr = err
				safeYield(*new(O), err)
			}
		}

		// Close result channel and wait for consumer to finish
		close(resultChan)
		<-yieldDone

		// Publish graph completion event
		publishCompletionEvent(ctx, runCfg.runID, runtimeErr)
	}
}

// pregelGraphAdapter adapts ExecutorConfig to the pregel.Graph interface.
type pregelGraphAdapter[I, O any] struct {
	cfg        *ExecutorConfig[I, O]
	runCfg     *runConfig
	bspState   *BSPState           // BSP-compliant state with read snapshots and write buffering
	safeYield  func(O, error) bool // Thread-safe yield via channel
	middleware []Middleware

	superstep          int
	checkpointInterval int
}

// RootVertices returns the entry points.
func (a *pregelGraphAdapter[I, O]) RootVertices() []string {
	return a.cfg.EntryPoints
}

// Outgoing returns the target nodes for a given node.
func (a *pregelGraphAdapter[I, O]) Outgoing(vertex string) []string {
	if node, ok := a.cfg.Nodes[vertex]; ok {
		return node.Targets
	}
	return nil
}

// VertexByName returns a vertex adapter for the given node.
func (a *pregelGraphAdapter[I, O]) VertexByName(name string) pregel.Vertex[*ExecutorConfig[I, O], Updates] {
	return &pregelVertexAdapter[I, O]{
		name:    name,
		adapter: a,
	}
}

// State returns the executor configuration.
func (a *pregelGraphAdapter[I, O]) State() *ExecutorConfig[I, O] {
	return a.cfg
}

func (a *pregelGraphAdapter[I, O]) managedValueDescriptors() []checkpoint.ManagedValueDescriptor {
	if a.runCfg.managedValues == nil {
		return nil
	}
	return a.runCfg.managedValues.descriptors()
}

// yieldListItems yields each item in a slice as an output.
// Uses SliceValue interface when available to avoid reflection.
// Lock-free: uses buffered channel for thread-safe parallel node execution.
func (a *pregelGraphAdapter[I, O]) yieldListItems(items any) {
	// Fast path: use SliceValue interface (no reflection)
	if sv, ok := items.(SliceValue); ok {
		sv.SliceIter(func(item any) bool {
			if o, ok := item.(O); ok {
				return a.safeYield(o, nil)
			}
			return true
		})
		return
	}

	// Slow path: fall back to reflection for untyped slices
	val := reflect.ValueOf(items)
	if val.Kind() != reflect.Slice {
		return
	}

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		if o, ok := item.(O); ok {
			a.safeYield(o, nil)
		}
	}
}

// yieldValue yields a single value as output.
// Lock-free: uses buffered channel for thread-safe parallel node execution.
func (a *pregelGraphAdapter[I, O]) yieldValue(val any) {
	if o, ok := val.(O); ok {
		a.safeYield(o, nil)
	}
}

// yieldUpdates yields output values from updates if the output key is present.
// Uses the OutputIsList flag determined at graph build time to avoid runtime reflection.
func (a *pregelGraphAdapter[I, O]) yieldUpdates(updates Updates) {
	if a.cfg.OutputKey == "" {
		return
	}

	val, ok := updates[a.cfg.OutputKey]
	if !ok {
		return
	}

	// Use build-time flag instead of runtime isSlice() check
	if a.cfg.OutputIsList {
		a.yieldListItems(val)
	} else {
		a.yieldValue(val)
	}
}

// checkInterrupt checks if an interrupt is needed and returns an error if so.
func (a *pregelGraphAdapter[I, O]) checkInterrupt(
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
		if a.runCfg.approvals == nil || a.runCfg.approvals[nodeName] == nil {
			return &InterruptError{NodeName: nodeName, Before: isBefore}
		}
	}
	return nil
}

// pregelVertexAdapter adapts a node to the pregel.Vertex interface.
type pregelVertexAdapter[I, O any] struct {
	name    string
	adapter *pregelGraphAdapter[I, O]
}

// Name returns the vertex name.
func (v *pregelVertexAdapter[I, O]) Name() string {
	return v.name
}

// Run executes the vertex computation.
func (v *pregelVertexAdapter[I, O]) Run(
	ctx context.Context,
	vctx pregel.VertexContext[*ExecutorConfig[I, O], Updates],
	incoming []pregel.Message[Updates],
) error {
	node, ok := v.adapter.cfg.Nodes[v.name]
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
	if icfg, hasInterrupt := v.adapter.cfg.InterruptsBefore[v.name]; hasInterrupt {
		if err := v.adapter.checkInterrupt(ctx, v.name, icfg, true); err != nil {
			nodeErr = err
			return err
		}
	}

	// Create BSP read view for node execution (reads from previous superstep snapshot)
	view := v.adapter.bspState.ReadView()

	// Add node name to context for middleware
	ctx = WithNodeName(ctx, v.name)

	// Add stream writer to context for intermediate streaming
	// This allows nodes to emit updates before they complete
	// Updates are published as events AND yielded as outputs (if they match output key)
	streamWriter := func(updates Updates) {
		// Publish state update event
		event.Publish(ctx, event.Event{
			Type:      event.EventStateUpdate,
			Node:      v.name,
			Timestamp: time.Now(),
			Data:      map[string]any{"updates": updates},
		})
		// Also yield to output stream (will only emit if output key is present)
		v.adapter.yieldUpdates(updates)
	}
	ctx = WithStreamWriter(ctx, streamWriter)

	// Apply middleware
	fn := node.Fn
	for i := len(v.adapter.middleware) - 1; i >= 0; i-- {
		fn = v.adapter.middleware[i](fn)
	}

	// Execute node
	cmd, err := fn(ctx, view)
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
	if icfg, hasInterrupt := v.adapter.cfg.InterruptsAfter[v.name]; hasInterrupt {
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
func (a *pregelGraphAdapter[I, O]) twoPhaseCommit(ctx context.Context, superstep int64) error {
	// Check if checkpointing is configured
	checkpointerEnabled := a.cfg.Checkpointer != nil
	runID := a.runCfg.runID
	if runID == "" {
		runID = a.cfg.RunID
	}
	checkpointerEnabled = checkpointerEnabled && runID != ""

	// Check if we should save based on interval
	shouldCheckpoint := checkpointerEnabled &&
		(a.checkpointInterval <= 0 || int(superstep)%a.checkpointInterval == 0)

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
			Committed:     false, // Mark as uncommitted - pending writes not yet applied
			Timestamp:     time.Now(),
			ManagedValues: a.managedValueDescriptors(),
		}

		if err := a.cfg.Checkpointer.Save(ctx, phase1Checkpoint); err != nil {
			if a.runCfg.failOnCheckpointErr {
				return fmt.Errorf("two-phase commit phase 1 failed at superstep %d: %w", superstep, err)
			}
			// Continue without checkpoint
		}
	}

	// BSP Barrier: Commit all buffered writes and create new read snapshot
	// After this, all writes from this superstep become visible to the next
	a.bspState.CommitBarrier()

	// Phase 2: Save checkpoint with Committed=true (after barrier commit)
	// This marks the transaction as complete - all pending writes have been applied
	// If a crash occurs after this, we skip re-applying writes on recovery
	if shouldCheckpoint {
		phase2Checkpoint := &checkpoint.Checkpoint{
			RunID:         runID,
			Superstep:     superstep,
			State:         a.bspState.Snapshot(), // Committed state AFTER barrier
			PendingWrites: nil,                   // No pending writes - all committed
			Committed:     true,                  // Mark as committed
			Timestamp:     time.Now(),
			ManagedValues: a.managedValueDescriptors(),
		}

		if err := a.cfg.Checkpointer.Save(ctx, phase2Checkpoint); err != nil {
			if a.runCfg.failOnCheckpointErr {
				return fmt.Errorf("two-phase commit phase 2 failed at superstep %d: %w", superstep, err)
			}
			// Continue - Phase 1 checkpoint can be used for recovery
		}
	}

	return nil
}
