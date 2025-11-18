package pregel

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/quota"
)

// Runtime orchestrates Pregel-style bulk-synchronous parallel (BSP) execution
// of a graph. It maintains the mailbox, aggregators, and superstep counter.
//
// Concurrency Model:
//   - Run() executes supersteps with configurable worker pool (MaxWorkers)
//   - Deliver() can be called concurrently with Run() to inject messages
//   - Multiple goroutines execute vertices in parallel within each superstep
//   - Mailbox and frontier access is synchronized via atomic operations
//
// Mutex Usage:
//
//   - aggMu: Protects aggregator state (aggregates, nextAggregates)
//
//   - Acquired in: recordAggregation(), snapshotAggregates(), finalizeAggregators()
//
//   - Lock duration: Short - only held during aggregator read/write
//
//   - frontierMu: Protects nextFrontier during concurrent updates
//
//   - Acquired in: recordDeliveries(), consumeNextFrontier()
//
//   - Lock duration: Very short - only held during map insert/swap
//
//   - Never held while executing vertex compute functions
//
// Memory Management:
//   - MaxMailboxSize option prevents unbounded mailbox growth
//   - Messages are dropped (with warning event) when mailbox limit is exceeded
//   - Combiner reduces message volume by merging messages for same target
//
// Execution Flow:
//  1. Initialize frontier from graph root nodes
//  2. For each superstep:
//     a. Execute vertices in parallel (worker pool)
//     - Each worker drains its own mailbox (parallel draining)
//     - Then executes the vertex computation
//     b. Collect sent messages and update frontier incrementally
//     c. Finalize aggregators
//  3. Repeat until frontier is empty or max iterations reached
//
// Performance Optimizations:
//   - Mailbox draining happens in parallel within the worker pool
//   - This eliminates the sequential draining bottleneck for distributed
//     message bus implementations (Redis, gRPC), providing 10-100x speedup
//   - Incremental frontier tracking avoids scanning entire mailbox each superstep
//   - Frontier is built as messages are sent, not by scanning pending messages
type Runtime[S any, M any] struct {
	graph Graph[S, M]
	opts  RuntimeOptions[S, M]

	messageBus MessageBus[M] // Pluggable message delivery backend

	aggMu          sync.Mutex // Protects aggregator state (see concurrency model above)
	aggregators    map[string]Aggregator
	aggregates     map[string]any // Current superstep aggregates (read-only for vertices)
	nextAggregates map[string]any // Next superstep aggregates (write-only during execution)

	// Incremental frontier tracking - avoids scanning mailbox
	frontierMu   sync.Mutex          // Protects nextFrontier during concurrent message sends
	nextFrontier map[string]struct{} // Vertices with pending messages for next superstep

	supersteps atomic.Int64
	vertices   atomic.Int64
	messages   atomic.Int64

	// Event emission for Run execution (thread-safe channel wrapper)
	eventChanMu sync.RWMutex
	eventChan   *safeEventChan[M]

	// Resource quota management (optional)
	quotaManager *quota.Manager
}

// initializeAggregators creates and initializes aggregator maps from the provided configuration.
// Filters out nil or empty-named aggregators and initializes each with its Zero value.
// Returns nil for all maps if no valid aggregators are found.
func initializeAggregators(optsAggregators map[string]Aggregator) (
	aggregators map[string]Aggregator,
	aggregates map[string]any,
	nextAggregates map[string]any,
) {
	if len(optsAggregators) == 0 {
		return nil, nil, nil
	}

	aggregators = make(map[string]Aggregator, len(optsAggregators))
	aggregates = make(map[string]any, len(optsAggregators))
	nextAggregates = make(map[string]any, len(optsAggregators))

	for name, agg := range optsAggregators {
		if name == "" || agg == nil {
			continue
		}
		aggregators[name] = agg
		aggregates[name] = agg.Zero()
		nextAggregates[name] = agg.Zero()
	}

	// If all aggregators were filtered out, return nil
	if len(aggregators) == 0 {
		return nil, nil, nil
	}

	return aggregators, aggregates, nextAggregates
}

// NewRuntime creates a new runtime for the given graph.
// Returns an error if graph is nil or invalid.
func NewRuntime[S any, M any](graph Graph[S, M], optFns ...RuntimeOption[S, M]) (*Runtime[S, M], error) {
	if graph == nil {
		return nil, ErrGraphRequired
	}

	opts := defaultRuntimeOptions[S, M]()
	for _, fn := range optFns {
		if fn != nil {
			fn(&opts)
		}
	}
	if opts.MaxWorkers <= 0 {
		opts.MaxWorkers = defaultRuntimeOptions[S, M]().MaxWorkers
	}
	if opts.InitialSuperstep < 0 {
		opts.InitialSuperstep = 0
	}

	// Initialize aggregators
	aggregators, aggregates, nextAggregates := initializeAggregators(opts.Aggregators)

	// Create message bus (use provided bus or default to in-memory)
	var messageBus MessageBus[M]
	if opts.MessageBus != nil {
		messageBus = opts.MessageBus
	} else {
		messageBus = NewInMemoryMessageBus[M](opts.MaxMailboxSize, opts.Combiner)
	}

	// Create quota manager if configured
	var quotaManager *quota.Manager
	if opts.QuotaConfig != nil {
		quotaManager = quota.New(quota.Config{
			MaxMemoryBytes:   opts.QuotaConfig.MaxMemoryBytes,
			MaxGoroutines:    opts.QuotaConfig.MaxGoroutines,
			MaxExecutionTime: opts.QuotaConfig.MaxExecutionTime,
		})
	}

	runtime := &Runtime[S, M]{
		graph:          graph,
		opts:           opts,
		messageBus:     messageBus,
		aggregators:    aggregators,
		aggregates:     aggregates,
		nextAggregates: nextAggregates,
		nextFrontier:   make(map[string]struct{}),
		quotaManager:   quotaManager,
	}
	runtime.SetSuperstep(opts.InitialSuperstep)
	return runtime, nil
}

// MustNewRuntime creates a new runtime for the given graph.
// Panics if graph is nil or invalid. Use this in tests or when you're certain inputs are valid.
func MustNewRuntime[S any, M any](graph Graph[S, M], optFns ...RuntimeOption[S, M]) *Runtime[S, M] {
	runtime, err := NewRuntime(graph, optFns...)
	if err != nil {
		panic(err)
	}
	return runtime
}

// Deliver injects messages into the mailbox and schedules their targets for execution.
// It is safe to call concurrently with Run.
//
// Blocks if mailbox is full (backpressure) until space is available or context deadline exceeded.
// Pass context.Background() for unlimited wait or context.WithTimeout() for bounded wait.
func (r *Runtime[S, M]) Deliver(ctx context.Context, messages ...Message[M]) error {
	if len(messages) == 0 {
		return nil
	}
	return r.recordDeliveries(ctx, messages)
}

// Stats snapshots the current runtime metrics.
func (r *Runtime[S, M]) Stats() RuntimeStats {
	return RuntimeStats{
		Supersteps: r.supersteps.Load(),
		Vertices:   r.vertices.Load(),
		Messages:   r.messages.Load(),
	}
}

// SetSuperstep seeds the superstep counter. Useful when resuming from a
// persisted snapshot. It overwrites any previously recorded value.
func (r *Runtime[S, M]) SetSuperstep(superstep int64) {
	if superstep < 0 {
		superstep = 0
	}
	r.supersteps.Store(superstep)
}

// CurrentSuperstep returns the superstep counter without exposing atomics.
func (r *Runtime[S, M]) CurrentSuperstep() int64 {
	return r.supersteps.Load()
}

func (r *Runtime[S, M]) snapshotAggregates() map[string]any {
	r.aggMu.Lock()
	defer r.aggMu.Unlock()
	if len(r.aggregates) == 0 {
		return nil
	}
	snapshot := make(map[string]any, len(r.aggregates))
	maps.Copy(snapshot, r.aggregates)
	return snapshot
}

// Aggregates returns a snapshot of the current aggregated values.
// This is typically called after Run() completes to retrieve final aggregates.
func (r *Runtime[S, M]) Aggregates() map[string]any {
	return r.snapshotAggregates()
}

func (r *Runtime[S, M]) recordAggregation(name string, value any) error {
	if len(r.aggregators) == 0 {
		return fmt.Errorf("%w", ErrAggregatorsNotConfigured)
	}
	r.aggMu.Lock()
	defer r.aggMu.Unlock()
	agg, ok := r.aggregators[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownAggregator, name)
	}
	if r.nextAggregates == nil {
		r.nextAggregates = make(map[string]any, len(r.aggregators))
	}
	current := r.nextAggregates[name]
	if current == nil {
		current = agg.Zero()
	}
	r.nextAggregates[name] = agg.Aggregate(current, value)
	return nil
}

func (r *Runtime[S, M]) finalizeAggregators() {
	if len(r.aggregators) == 0 {
		return
	}
	r.aggMu.Lock()
	defer r.aggMu.Unlock()
	if r.aggregates == nil {
		r.aggregates = make(map[string]any, len(r.aggregators))
	}
	if r.nextAggregates == nil {
		r.nextAggregates = make(map[string]any, len(r.aggregators))
	}
	for name, agg := range r.aggregators {
		next := r.nextAggregates[name]
		if next == nil {
			// No aggregation in this superstep - preserve previous value
			next = r.aggregates[name]
			if next == nil {
				next = agg.Zero()
			}
		}
		r.aggregates[name] = next
		r.nextAggregates[name] = nil // Reset for next superstep
	}
}

// Run executes all supersteps until the computation quiesces.
// Returns an iterator that yields events as the computation progresses.
//
// ERROR HANDLING:
//   - Fatal errors (context canceled, max iterations, quota exceeded) are returned
//     in the second return value (err) following Go iterator convention
//   - When err != nil, iteration stops and no more events are yielded
//
// Example:
//
//	for evt, err := range runtime.Run(ctx) {
//	    if err != nil {
//	        return fmt.Errorf("BSP execution failed: %w", err)
//	    }
//	    // Process event...
//	}
func (r *Runtime[S, M]) Run(ctx context.Context) iter.Seq2[Event[M], error] {
	return func(yield func(Event[M], error) bool) {
		// Create thread-safe event channel wrapper
		r.eventChanMu.Lock()
		r.eventChan = newSafeEventChan[M](DefaultEventChanBufferSize)
		ch := r.eventChan.Chan()
		r.eventChanMu.Unlock()

		doneChan := make(chan struct{})

		defer func() {
			// Close the safe channel wrapper (prevents "send on closed channel" panics)
			r.eventChanMu.Lock()
			if r.eventChan != nil {
				r.eventChan.Close()
				r.eventChan = nil
			}
			r.eventChanMu.Unlock()
		}()

		// Start goroutine that actually executes the runtime
		go func() {
			defer func() {
				// Close the underlying channel when execution completes
				r.eventChanMu.RLock()
				if r.eventChan != nil {
					r.eventChan.Close()
				}
				r.eventChanMu.RUnlock()
				close(doneChan)
			}()
			r.execute(ctx)
		}()

		// Yield events from single goroutine (satisfies iter.Seq2 contract)
		for eoe := range ch {
			if !yield(eoe.event, eoe.err) {
				// Consumer stopped iteration - drain remaining events to prevent deadlock
				go func() {
					//nolint:revive // Need to drain channel to prevent goroutine leak
					for range ch {
					}
				}()
				<-doneChan // Wait for execution to complete
				return
			}
		}
	}
}

// execute runs the actual computation.
func (r *Runtime[S, M]) execute(ctx context.Context) {
	logger := logging.FromContext(ctx)
	frontier := r.initialFrontier()
	superstep := r.supersteps.Load()
	iterationCount := int64(0)

	// Start quota manager if configured
	if r.quotaManager != nil {
		r.quotaManager.Start()
	}

	logger.Info("pregel runtime starting",
		"initial_frontier_size", len(frontier),
		"initial_superstep", superstep,
		"max_workers", r.opts.MaxWorkers,
		"max_iterations", r.opts.MaxIterations)

	var err error
	for len(frontier) > 0 {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			logger.Warn("pregel runtime canceled", "superstep", superstep, "error", err)
			r.emitEvent(Event[M]{Superstep: superstep}, err)
			return
		}

		// Check resource quotas
		if r.quotaManager != nil {
			// Check memory
			if err := r.quotaManager.CheckMemory(ctx); err != nil {
				logger.Error("memory quota exceeded", "superstep", superstep, "error", err)
				r.emitEvent(Event[M]{Superstep: superstep}, err)
				return
			}
			// Check execution time
			if err := r.quotaManager.CheckTime(ctx); err != nil {
				logger.Error("time quota exceeded", "superstep", superstep, "error", err)
				r.emitEvent(Event[M]{Superstep: superstep}, err)
				return
			}
		}

		// Check max iterations limit (if configured)
		if r.opts.MaxIterations > 0 && iterationCount >= int64(r.opts.MaxIterations) {
			logger.Warn("max iterations exceeded",
				"max_iterations", r.opts.MaxIterations,
				"superstep", superstep)
			r.emitEvent(Event[M]{Superstep: superstep}, ErrMaxIterationsExceeded)
			return
		}
		iterationCount++

		nextSuperstep := superstep + 1
		r.supersteps.Store(nextSuperstep)

		logger.Debug("starting superstep",
			"superstep", nextSuperstep,
			"frontier_size", len(frontier))

		if err := r.runSuperstep(ctx, frontier, nextSuperstep); err != nil {
			logger.Error("superstep execution failed",
				"superstep", nextSuperstep,
				"error", err)
			r.emitEvent(Event[M]{Superstep: nextSuperstep}, err)
			return
		}
		superstep = nextSuperstep

		// Call superstep completion callback (useful for checkpointing)
		if r.opts.OnSuperstepComplete != nil {
			r.opts.OnSuperstepComplete(ctx, superstep)
		}

		frontier, err = r.consumeNextFrontier()
		if err != nil {
			logger.Error("failed to consume next frontier",
				"superstep", superstep,
				"error", err)
			r.emitEvent(Event[M]{Superstep: superstep}, err)
			return
		}

		logger.Debug("superstep completed",
			"superstep", superstep,
			"next_frontier_size", len(frontier))
	}

	logger.Info("pregel runtime completed",
		"total_supersteps", superstep,
		"total_vertices", r.vertices.Load(),
		"total_messages", r.messages.Load())
}

func (r *Runtime[S, M]) initialFrontier() map[string]struct{} {
	frontier := make(map[string]struct{})

	// Add root nodes
	for _, name := range r.graph.RootNodes() {
		frontier[name] = struct{}{}
	}

	// Add nodes with pending messages (fallback to message bus for initial setup)
	// This handles cases where messages were delivered before Run() started
	pending, err := r.messageBus.Pending()
	if err == nil {
		for _, name := range pending {
			frontier[name] = struct{}{}
		}
	}

	return frontier
}

func (r *Runtime[S, M]) consumeNextFrontier() (map[string]struct{}, error) {
	// Swap frontier atomically - this is much faster than scanning the mailbox
	r.frontierMu.Lock()
	frontier := r.nextFrontier
	r.nextFrontier = make(map[string]struct{})
	r.frontierMu.Unlock()

	if len(frontier) == 0 {
		return nil, nil // No error, just no work
	}

	return frontier, nil
}

// runSuperstep executes a single superstep for all vertices in the frontier.
// The function orchestrates parallel execution with configurable worker pool size.
// Mailbox draining now happens in parallel within the worker pool for optimal
// performance in distributed deployments.
func (r *Runtime[S, M]) runSuperstep(ctx context.Context, frontier map[string]struct{}, superstep int64) error {
	if len(frontier) == 0 {
		return nil
	}

	// Setup execution context
	superCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	names := r.sortedFrontierNames(frontier)

	// Execute vertices in parallel (draining happens inside worker loop)
	if err := r.executeVerticesParallel(superCtx, names, superstep, cancel); err != nil {
		return err
	}

	// Finalize aggregators after all vertices complete
	r.finalizeAggregators()

	return ctx.Err()
}

// sortedFrontierNames extracts and sorts vertex names from the frontier.
func (r *Runtime[S, M]) sortedFrontierNames(frontier map[string]struct{}) []string {
	names := make([]string, 0, len(frontier))
	for name := range frontier {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// executeVerticesParallel executes all vertices in parallel using a worker pool.
// Each worker drains its mailbox and executes the vertex in parallel, eliminating
// the sequential draining bottleneck for distributed deployments.
func (r *Runtime[S, M]) executeVerticesParallel(
	ctx context.Context,
	names []string,
	superstep int64,
	cancel context.CancelFunc,
) error {
	workers := r.calculateWorkerCount(len(names))
	tasks := make(chan string)

	var wg sync.WaitGroup
	var once sync.Once
	var runErr error

	recordErr := func(err error) {
		if err == nil {
			return
		}
		once.Do(func() {
			runErr = err
			cancel()
		})
	}

	// Start worker pool
	r.startWorkerPool(ctx, &wg, workers, tasks, superstep, recordErr)

	// Schedule tasks
	r.scheduleTasks(ctx, tasks, names)

	// Wait for completion
	wg.Wait()

	return runErr
}

// calculateWorkerCount determines the optimal number of workers based on configuration and frontier size.
func (r *Runtime[S, M]) calculateWorkerCount(frontierSize int) int {
	workers := r.opts.MaxWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > frontierSize {
		workers = frontierSize
	}
	return workers
}

// startWorkerPool starts the configured number of worker goroutines.
func (r *Runtime[S, M]) startWorkerPool(
	ctx context.Context,
	wg *sync.WaitGroup,
	workers int,
	tasks <-chan string,
	superstep int64,
	recordErr func(error),
) {
	for i := 0; i < workers; i++ {
		// Acquire goroutine quota before spawning
		if r.quotaManager != nil {
			if err := r.quotaManager.AcquireGoroutine(ctx); err != nil {
				recordErr(fmt.Errorf("goroutine quota exceeded: %w", err))
				return
			}
		}

		wg.Add(1)
		go func() {
			defer func() {
				// Release goroutine quota when done
				if r.quotaManager != nil {
					r.quotaManager.ReleaseGoroutine()
				}
			}()
			r.workerLoop(ctx, wg, tasks, superstep, recordErr)
		}()
	}
}

// workerLoop is the main loop for a worker goroutine.
// Each worker drains the mailbox for its assigned vertex in parallel,
// then executes the vertex. This eliminates the sequential draining
// bottleneck for distributed message bus implementations.
func (r *Runtime[S, M]) workerLoop(
	ctx context.Context,
	wg *sync.WaitGroup,
	tasks <-chan string,
	superstep int64,
	recordErr func(error),
) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case name, ok := <-tasks:
			if !ok {
				return
			}

			// Drain mailbox in parallel (each worker drains its own)
			incoming, err := r.drainMailbox(name)
			if err != nil {
				recordErr(fmt.Errorf("failed to drain mailbox for %s: %w", name, err))
				return
			}

			// Execute vertex with drained messages
			if err := r.executeVertex(ctx, name, incoming, superstep); err != nil {
				recordErr(err)
				return
			}
		}
	}
}

// scheduleTasks sends all tasks to the worker pool, respecting context cancellation.
func (r *Runtime[S, M]) scheduleTasks(ctx context.Context, tasks chan<- string, names []string) {
	defer close(tasks)
	for _, name := range names {
		select {
		case <-ctx.Done():
			return
		case tasks <- name:
		}
	}
}

func (r *Runtime[S, M]) executeVertex(ctx context.Context, name string, incoming []Message[M], superstep int64) (err error) {
	// Don't check ctx.Err() here - let the node handle it and wrap appropriately
	node := r.graph.NodeByName(name)
	if node == nil {
		err := fmt.Errorf("superstep %d: node %q: %w", superstep, name, ErrUnknownNode)
		r.emitEvent(Event[M]{Node: name, Superstep: superstep}, err)
		return err
	}

	// Apply per-vertex timeout if configured
	vertexCtx := ctx
	var cancel context.CancelFunc
	if r.opts.VertexTimeout > 0 {
		vertexCtx, cancel = context.WithTimeout(ctx, r.opts.VertexTimeout)
		defer cancel()
	}

	r.vertices.Add(1)
	var sent []Message[M]
	send := func(msg Message[M]) {
		sent = append(sent, msg)
	}

	state := r.graph.State()
	aggregates := r.snapshotAggregates()
	var aggregateFn func(string, any) error
	if len(r.aggregators) > 0 {
		aggregateFn = r.recordAggregation
	}
	vertex := VertexContext[S, M]{
		State:      state,
		Send:       send,
		Aggregate:  aggregateFn,
		Aggregates: aggregates,
	}
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			recovered := fmt.Errorf("superstep %d: node %q: %w: %v", superstep, name, ErrNodePanicked, rec)
			r.emitEvent(Event[M]{Node: name, Superstep: superstep, Diagnostics: stack}, recovered)
			err = recovered
		}
	}()

	runErr := node.Run(vertexCtx, vertex, incoming)
	if runErr != nil {
		err = fmt.Errorf("superstep %d: node %q failed: %w", superstep, name, runErr)
		r.emitEvent(Event[M]{Node: name, Superstep: superstep}, err)
		return err
	}

	if len(sent) > 0 {
		r.messages.Add(int64(len(sent)))
		// Use context from executeVertex to support backpressure
		if deliverErr := r.recordDeliveries(ctx, sent); deliverErr != nil {
			// If message delivery fails due to backpressure/timeout, treat as error
			err = fmt.Errorf("superstep %d: node %q: failed to deliver messages: %w", superstep, name, deliverErr)
			r.emitEvent(Event[M]{Node: name, Superstep: superstep}, err)
			return err
		}
	}

	r.emitEvent(Event[M]{Node: name, Superstep: superstep}, nil)
	return nil
}

func (r *Runtime[S, M]) recordDeliveries(ctx context.Context, msgs []Message[M]) error {
	if len(msgs) == 0 {
		return nil
	}

	// Send all messages at once with context for backpressure
	err := r.messageBus.Send(ctx, msgs)
	if err != nil {
		// Emit error event
		r.emitEvent(Event[M]{}, fmt.Errorf("failed to deliver messages: %w", err))
		return err
	}

	// Update frontier incrementally - record which vertices have pending messages
	r.frontierMu.Lock()
	for _, msg := range msgs {
		if msg.To != "" {
			r.nextFrontier[msg.To] = struct{}{}
		}
	}
	r.frontierMu.Unlock()

	return nil
}

func (r *Runtime[S, M]) drainMailbox(node string) ([]Message[M], error) {
	msgs, err := r.messageBus.Receive(node)
	if err != nil {
		err = fmt.Errorf("message bus receive failed for node %s: %w", node, err)
		r.emitEvent(Event[M]{}, err)
		return nil, fmt.Errorf("drain mailbox for %s: %w", node, err)
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	return msgs, nil
}

// emitEvent safely sends an event to the channel (if Run is active).
// Can be called concurrently from multiple worker goroutines.
// Returns true if the event was sent successfully, false if the channel is closed
// or the send timed out. Failed sends are not errors - they occur during normal
// shutdown or when the consumer is slow.
func (r *Runtime[S, M]) emitEvent(event Event[M], err error) bool {
	r.eventChanMu.RLock()
	defer r.eventChanMu.RUnlock()

	if r.eventChan == nil {
		return false
	}
	return r.eventChan.Send(event, err)
}
