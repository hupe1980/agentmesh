package pregel

import (
	"context"
	"fmt"
	"maps"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
)

// Runtime executes a Pregel-style computation over any graph.
//
// Concurrency Model:
//   - Run() executes supersteps with configurable worker pool (MaxWorkers)
//   - Deliver() can be called concurrently with Run() to inject messages
//   - Multiple goroutines execute vertices in parallel within each superstep
//   - Mailbox and frontier access is synchronized via mutex
//
// Mutex Usage:
//
//   - mu: Protects mailbox and nextFrontier during message delivery and drainage
//
//   - Acquired in: recordDeliveries(), drainMailbox(), initialFrontier()
//
//   - Lock duration: Short - only held during map operations
//
//   - Never held while executing vertex compute functions
//
//   - aggMu: Protects aggregator state (aggregates, nextAggregates)
//
//   - Acquired in: recordAggregation(), snapshotAggregates(), finalizeAggregators()
//
//   - Lock duration: Short - only held during aggregator read/write
//
//   - Independent of mu - can be acquired in any order
//
// Memory Management:
//   - MaxMailboxSize option prevents unbounded mailbox growth
//   - Messages are dropped (with warning event) when mailbox limit is exceeded
//   - Combiner reduces message volume by merging messages for same target
//
// Execution Flow:
//  1. Initialize frontier from graph root nodes
//  2. For each superstep:
//     a. Drain mailboxes for active vertices
//     b. Execute vertices in parallel (worker pool)
//     c. Collect sent messages and update frontier
//     d. Finalize aggregators
//  3. Repeat until frontier is empty or max iterations reached
type Runtime[S any, M any] struct {
	graph  PregelGraph[S, M]
	events chan StreamEvent[M]
	opts   RuntimeOptions[S, M]

	messageBus MessageBus[M] // Pluggable message delivery backend

	aggMu          sync.Mutex // Protects aggregator state (see concurrency model above)
	aggregators    map[string]Aggregator
	aggregates     map[string]any // Current superstep aggregates (read-only for vertices)
	nextAggregates map[string]any // Next superstep aggregates (write-only during execution)

	supersteps atomic.Int64
	vertices   atomic.Int64
	messages   atomic.Int64

	doneChan <-chan struct{} // Signals early termination request
}

// NewRuntime creates a new runtime for the given graph.
// Returns an error if graph is nil or invalid.
func NewRuntime[S any, M any](graph PregelGraph[S, M], events chan StreamEvent[M], optFns ...RuntimeOption[S, M]) (*Runtime[S, M], error) {
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
	var (
		aggregators    map[string]Aggregator
		aggregates     map[string]any
		nextAggregates map[string]any
	)
	if len(opts.Aggregators) > 0 {
		aggregators = make(map[string]Aggregator, len(opts.Aggregators))
		aggregates = make(map[string]any, len(opts.Aggregators))
		nextAggregates = make(map[string]any, len(opts.Aggregators))
		for name, agg := range opts.Aggregators {
			if name == "" || agg == nil {
				continue
			}
			aggregators[name] = agg
			aggregates[name] = agg.Zero()
			nextAggregates[name] = agg.Zero()
		}
		if len(aggregators) == 0 {
			aggregators = nil
			aggregates = nil
			nextAggregates = nil
		}
	}

	// Create message bus (use provided bus or default to in-memory)
	var messageBus MessageBus[M]
	if opts.MessageBus != nil {
		messageBus = opts.MessageBus
	} else {
		messageBus = NewInMemoryMessageBus[M](opts.MaxMailboxSize, opts.Combiner)
	}

	runtime := &Runtime[S, M]{
		graph:          graph,
		events:         events,
		opts:           opts,
		messageBus:     messageBus,
		aggregators:    aggregators,
		aggregates:     aggregates,
		nextAggregates: nextAggregates,
	}
	runtime.SetSuperstep(opts.InitialSuperstep)
	return runtime, nil
}

// MustNewRuntime creates a new runtime for the given graph.
// Panics if graph is nil or invalid. Use this in tests or when you're certain inputs are valid.
func MustNewRuntime[S any, M any](graph PregelGraph[S, M], events chan StreamEvent[M], optFns ...RuntimeOption[S, M]) *Runtime[S, M] {
	runtime, err := NewRuntime(graph, events, optFns...)
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
func (r *Runtime[S, M]) Run(ctx context.Context) error {
	frontier := r.initialFrontier()
	superstep := r.supersteps.Load()
	iterationCount := int64(0)

	for len(frontier) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Check max iterations limit (if configured)
		if r.opts.MaxIterations > 0 && iterationCount >= int64(r.opts.MaxIterations) {
			return ErrMaxIterationsExceeded
		}
		iterationCount++

		nextSuperstep := superstep + 1
		r.supersteps.Store(nextSuperstep)
		if err := r.runSuperstep(ctx, frontier, nextSuperstep); err != nil {
			return err
		}
		superstep = nextSuperstep

		// Call superstep completion callback (useful for checkpointing)
		if r.opts.OnSuperstepComplete != nil {
			r.opts.OnSuperstepComplete(superstep)
		}

		frontier = r.consumeNextFrontier()
	}

	// Return nil on successful completion (don't return ctx.Err() which might be non-nil)
	return nil
}

func (r *Runtime[S, M]) initialFrontier() map[string]struct{} {
	frontier := make(map[string]struct{})

	// Add root nodes
	for _, name := range r.graph.RootNodes() {
		frontier[name] = struct{}{}
	}

	// Add nodes with pending messages
	pending, err := r.messageBus.Pending()
	if err == nil {
		for _, name := range pending {
			frontier[name] = struct{}{}
		}
	}

	return frontier
}

func (r *Runtime[S, M]) consumeNextFrontier() map[string]struct{} {
	// Get vertices with pending messages from message bus
	pending, err := r.messageBus.Pending()
	if err != nil || len(pending) == 0 {
		return nil
	}

	frontier := make(map[string]struct{}, len(pending))
	for _, name := range pending {
		frontier[name] = struct{}{}
	}
	return frontier
}

//nolint:gocyclo // Superstep execution requires coordinating many runtime conditions
func (r *Runtime[S, M]) runSuperstep(ctx context.Context, frontier map[string]struct{}, superstep int64) error {
	if len(frontier) == 0 {
		return nil
	}

	superCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	names := make([]string, 0, len(frontier))
	for name := range frontier {
		names = append(names, name)
	}
	sort.Strings(names)

	incoming := make(map[string][]Message[M], len(names))
	for _, name := range names {
		incoming[name] = r.drainMailbox(name)
	}

	workers := r.opts.MaxWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > len(names) {
		workers = len(names)
	}

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

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-superCtx.Done():
					return
				case name, ok := <-tasks:
					if !ok {
						return
					}
					if err := r.executeVertex(superCtx, name, incoming[name], superstep); err != nil {
						recordErr(err)
					}
				}
			}
		}()
	}

schedule:
	for _, name := range names {
		select {
		case <-superCtx.Done():
			break schedule
		case tasks <- name:
		}
	}
	close(tasks)
	wg.Wait()

	if runErr != nil {
		return runErr
	}

	r.finalizeAggregators()

	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Runtime[S, M]) executeVertex(ctx context.Context, name string, incoming []Message[M], superstep int64) (err error) {
	// Don't check ctx.Err() here - let the node handle it and wrap appropriately
	node := r.graph.NodeByName(name)
	if node == nil {
		err := fmt.Errorf("superstep %d: node %q: %w", superstep, name, ErrUnknownNode)
		r.emitEvent(StreamEvent[M]{Node: name, Superstep: superstep, Error: err})
		return err
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
			r.emitEvent(StreamEvent[M]{Node: name, Superstep: superstep, Diagnostics: stack, Error: recovered})
			err = recovered
		}
	}()

	runErr := node.Run(ctx, vertex, incoming)
	if runErr != nil {
		err = fmt.Errorf("superstep %d: node %q failed: %w", superstep, name, runErr)
		r.emitEvent(StreamEvent[M]{Node: name, Superstep: superstep, Error: err})
		return err
	}

	if len(sent) > 0 {
		r.messages.Add(int64(len(sent)))
		// Use context from executeVertex to support backpressure
		if deliverErr := r.recordDeliveries(ctx, sent); deliverErr != nil {
			// If message delivery fails due to backpressure/timeout, treat as error
			err = fmt.Errorf("superstep %d: node %q: failed to deliver messages: %w", superstep, name, deliverErr)
			r.emitEvent(StreamEvent[M]{Node: name, Superstep: superstep, Error: err})
			return err
		}
		r.graph.Update(name, nil, sent)
	}

	r.emitEvent(StreamEvent[M]{Node: name, Superstep: superstep})
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
		r.emitEvent(StreamEvent[M]{
			Error: fmt.Errorf("failed to deliver messages: %w", err),
		})
		return err
	}

	return nil
}

func (r *Runtime[S, M]) drainMailbox(node string) []Message[M] {
	msgs, err := r.messageBus.Receive(node)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	return msgs
}

func (r *Runtime[S, M]) emitEvent(event StreamEvent[M]) {
	if r.events == nil {
		return
	}

	// Check if early termination was requested
	if r.doneChan != nil {
		select {
		case <-r.doneChan:
			// Early termination - don't block on event emission
			return
		default:
		}
	}

	if event.Error == nil {
		select {
		case r.events <- event:
		case <-r.doneChan:
			// Early termination requested while trying to send
			return
		default:
			// Channel full, skip non-error event
		}
		return
	}

	// For error events, try harder to deliver
	select {
	case r.events <- event:
		return
	case <-r.doneChan:
		// Early termination requested
		return
	default:
	}

	// Last resort: spawn goroutine for error event
	// but respect done channel
	go func() {
		select {
		case r.events <- event:
		case <-r.doneChan:
		}
	}()
}

// SetDoneChannel configures the done channel for early termination detection.
// This allows the runtime to stop emitting events when the consumer has stopped listening.
func (r *Runtime[S, M]) SetDoneChannel(done <-chan struct{}) {
	r.doneChan = done
}
