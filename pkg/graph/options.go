package graph

import (
	"fmt"
	"runtime"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// Aggregator is defined in pregel.Aggregator.
// Import pregel.Aggregator directly to implement custom aggregators.
// See aggregators.go for built-in implementations (SumAggregator, MaxAggregator, etc.).

// SchedulingMessage describes an activation message between graph vertices.
// Used in distributed scheduling to propagate computation across nodes.
type SchedulingMessage struct {
	From string
	To   string
}

// Combiner merges multiple scheduling messages for the same target.
// Used to optimize message passing by reducing redundant activations.
type Combiner func(existing, incoming SchedulingMessage) SchedulingMessage

type runOptions struct {
	maxConcurrency        int
	initialSuperstep      int64
	maxIterations         int
	maxMessages           int // Maximum number of messages to retain (0 = unlimited)
	eventBufferSize       int // Size of event channel for streaming (default = 100)
	aggregators           map[string]pregel.Aggregator
	combiner              Combiner
	messageBus            pregel.MessageBus[ChannelMessage] // Custom message bus for distributed execution
	checkpointer          Checkpointer                      // Checkpoint storage backend
	checkpointInterval    int                               // Save every N supersteps (0 = every superstep)
	autoRestore           bool                              // Automatically restore from last checkpoint
	failOnCheckpointError bool                              // Fail execution on checkpoint errors (default: false, just log)
	logger                logging.Logger                    // Logger for observability (attached to context and used for instrumentation)
	tracer                trace.Provider                    // Trace provider for observability (attached to context and used for instrumentation)
	metricsProvider       metrics.Provider                  // Metrics provider for observability (attached to context and used for instrumentation)
	runID                 string                            // Unique identifier for this execution run
	resume                bool                              // Resume from checkpoint
	resumeFrom            int64                             // Superstep to resume from (0 = most recent)
}

type RunOption func(*runOptions)

func defaultRunOptions() runOptions {
	return runOptions{
		maxConcurrency:  runtime.NumCPU(),
		maxMessages:     0,   // Unlimited by default
		eventBufferSize: 100, // Default buffer size for event channel
		// Providers default to nil - noop implementations will be used in compiled_graph.go
	}
}

// WithMaxConcurrency sets the maximum number of nodes that can execute in parallel.
// Defaults to runtime.NumCPU(). Higher values may improve throughput for I/O-bound
// nodes but increase memory usage.
//
// Example:
//
//	compiled.Invoke(ctx, messages, graph.WithMaxConcurrency(8))
func WithMaxConcurrency(n int) RunOption {
	return func(opts *runOptions) {
		if n > 0 {
			opts.maxConcurrency = n
		}
	}
}

// WithInitialSuperstep sets the starting superstep number for execution.
// Primarily used for debugging or when resuming from a specific point.
// Negative values are clamped to 0.
//
// Example:
//
//	compiled.Invoke(ctx, messages, graph.WithInitialSuperstep(10))
func WithInitialSuperstep(superstep int64) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		if superstep < 0 {
			superstep = 0
		}
		opts.initialSuperstep = superstep
	}
}

// WithAggregators configures global aggregators for distributed reductions.
// Aggregators collect values from all nodes in each superstep and make the
// result available in the next superstep via state.AggregatesSnapshot().
//
// Example:
//
//	compiled.Invoke(ctx, messages, graph.WithAggregators(map[string]pregel.Aggregator{
//	    "total_cost": &graph.SumAggregator{},
//	    "max_priority": &graph.MaxAggregator{},
//	}))
func WithAggregators(aggregators map[string]pregel.Aggregator) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		if len(aggregators) == 0 {
			opts.aggregators = nil
			return
		}
		aggCopy := make(map[string]pregel.Aggregator, len(aggregators))
		for name, agg := range aggregators {
			if name == "" || agg == nil {
				continue
			}
			aggCopy[name] = agg
		}
		opts.aggregators = aggCopy
	}
}

// WithCombiner sets a function to merge multiple messages targeting the same node.
// This optimization can reduce redundant activations in highly connected graphs.
// The combiner receives existing and incoming messages and returns the merged result.
//
// Example:
//
//	compiled.Invoke(ctx, messages, graph.WithCombiner(func(existing, incoming SchedulingMessage) SchedulingMessage {
//	    // Custom merge logic
//	    return incoming  // Simple: last message wins
//	}))
func WithCombiner(combiner Combiner) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.combiner = combiner
	}
}

// WithMaxIterations sets the maximum number of supersteps allowed before terminating execution.
// This prevents infinite loops in cyclic graphs. A value <= 0 means unlimited (default).
// This is critical for production use with agent feedback loops.
func WithMaxIterations(n int) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.maxIterations = n
	}
}

// WithMaxMessages limits the number of messages retained in state.
// When the limit is reached, oldest messages are discarded.
// Use 0 for unlimited (default). Recommended: 100-1000 for long-running workflows.
func WithMaxMessages(n int) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		if n >= 0 {
			opts.maxMessages = n
		}
	}
}

// WithEventBufferSize sets the size of the event channel buffer for streaming.
// A larger buffer reduces backpressure when consumers are slow but uses more memory.
// Default is 100. Recommended: 10-1000 depending on event frequency and consumer speed.
//
// Use this option to tune for different workload characteristics:
//   - Fast consumers with limited memory: smaller buffer (10-50)
//   - Slow consumers or high event rate: larger buffer (200-1000)
//
// Example:
//
//	stream, _ := compiled.Stream(ctx, messages, graph.WithEventBufferSize(500))
func WithEventBufferSize(size int) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		if size > 0 {
			opts.eventBufferSize = size
		}
	}
}

// WithPregelMessageBus sets a custom message bus for distributed graph execution.
// This enables multi-process or multi-node execution of the graph using a shared
// message delivery backend (e.g., Redis, Kafka).
//
// The message bus handles communication between vertices during Pregel-style
// iterative computation. If not provided, an in-memory message bus is used.
//
// Example with Redis:
//
//	bus := redis.NewMessageBus[graph.ChannelMessage]("localhost:6379", "", 0, &redis.Options{
//	    Namespace: "my-graph-execution",
//	})
//	defer bus.Close()
//	compiled.Invoke(ctx, messages, graph.WithPregelMessageBus(bus))
func WithPregelMessageBus(bus pregel.MessageBus[ChannelMessage]) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.messageBus = bus
	}
}

// =============================================================================
// Checkpoint Options
// =============================================================================

// =============================================================================
// Checkpoint Options
// =============================================================================

// Re-export checkpoint types for convenience
type Checkpoint = checkpoint.Checkpoint
type Checkpointer = checkpoint.Checkpointer
type CheckpointConfig = checkpoint.Config

// WithCheckpointer enables automatic checkpointing with the given storage backend.
// Checkpoints are saved after each superstep by default.
//
// Example:
//
//	checkpointer := checkpoint.NewInMemoryCheckpointer()
//	compiled.Invoke(ctx, messages,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("user-123-session-456"),
//	)
func WithCheckpointer(checkpointer Checkpointer) RunOption {
	return func(opts *runOptions) {
		if opts == nil || checkpointer == nil {
			return
		}
		opts.checkpointer = checkpointer
	}
}

// WithCheckpointConfig provides fine-grained control over checkpointing behavior.
//
// Example:
//
//	compiled.Invoke(ctx, messages,
//	    graph.WithCheckpointConfig(graph.CheckpointConfig{
//	        Checkpointer:  checkpointer,
//	        SaveInterval:  5,     // Save every 5 supersteps
//	        AutoRestore:   true,  // Resume from last checkpoint
//	    }),
//	    graph.WithRunID("long-running-workflow"),
//	)
func WithCheckpointConfig(config CheckpointConfig) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.checkpointer = config.Checkpointer
		opts.checkpointInterval = config.SaveInterval
		opts.autoRestore = config.AutoRestore
	}
}

// WithRunID sets the unique identifier for this execution run.
// Required when using checkpointing to identify which execution to save/restore.
//
// Best practices:
//   - Use user ID + session ID for user-facing workflows
//   - Use UUID for background jobs
//   - Include version/variant for A/B testing
//
// Example:
//
//	graph.WithRunID("user-123-session-abc")
//	graph.WithRunID("batch-job-2024-11-04-uuid")
//	graph.WithRunID("experiment-variant-a-run-42")
func WithRunID(runID string) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.runID = runID
	}
}

// WithResumeFromSuperstep resumes execution from a specific superstep.
// Loads the checkpoint at that superstep and continues from there.
// If superstep is 0, resumes from the most recent checkpoint.
//
// Example:
//
//	// Resume from most recent
//	compiled.Invoke(ctx, nil,
//	    graph.WithRunID("user-123"),
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithResumeFromSuperstep(0),
//	)
//
//	// Resume from specific point (time-travel)
//	compiled.Invoke(ctx, nil,
//	    graph.WithRunID("user-123"),
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithResumeFromSuperstep(5),
//	)
func WithResumeFromSuperstep(superstep int64) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.resumeFrom = superstep
		opts.resume = true
	}
}

// WithFailOnCheckpointError configures whether checkpoint save errors should
// fail the entire graph execution or just be logged as warnings.
//
// By default (false), checkpoint errors are logged but don't stop execution.
// This allows the workflow to continue even if checkpoint storage is temporarily
// unavailable. Set to true for critical workflows where checkpoint integrity
// is required.
//
// Example:
//
//	// Fail immediately if checkpoints can't be saved
//	graph.WithFailOnCheckpointError(true)
//
//	// Log checkpoint errors but continue execution (default)
//	graph.WithFailOnCheckpointError(false)
func WithFailOnCheckpointError(fail bool) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.failOnCheckpointError = fail
	}
}

// WithLogger configures a custom logger for observability.
// The logger is automatically attached to the context passed to all node RunFuncs
// and used for automatic instrumentation. Nodes can retrieve it using logging.FromContext(ctx).
//
// Example:
//
//	logger := logging.NewSlogAdapter(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
//	compiled.Invoke(ctx, messages, graph.WithLogger(logger))
//
// In node RunFunc:
//
//	func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
//	    log := logging.FromContext(ctx) // Retrieves the configured logger
//	    log.Info("Processing node", "name", "my_node")
//	    // ...
//	}
func WithLogger(logger logging.Logger) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.logger = logger
	}
}

// WithTracer configures a trace provider for observability.
// The provider is automatically attached to the context passed to all node RunFuncs
// and used for automatic span creation. Nodes can retrieve it using trace.FromContext(ctx).
//
// Example:
//
//	import "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"
//
//	traceProvider := opentelemetry.NewProvider(
//	    opentelemetry.WithEndpoint("http://jaeger:4318"),
//	    opentelemetry.WithServiceName("my-service"),
//	)
//	compiled.Invoke(ctx, messages, graph.WithTracer(traceProvider))
//
// In node RunFunc:
//
//	func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
//	    tp := trace.FromContext(ctx)
//	    tracer := tp.Tracer("my-service")
//	    ctx, span := tracer.Start(ctx, "operation")
//	    defer span.End(nil)
//	    // ...
//	}
func WithTracer(tracer trace.Provider) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.tracer = tracer
	}
}

// WithMetrics configures a metrics provider for observability.
// The provider is automatically attached to the context passed to all node RunFuncs
// and used for automatic metrics recording. Nodes can retrieve it using metrics.FromContext(ctx).
//
// Example:
//
//	import "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"
//
//	metricsProvider := opentelemetry.NewMetricsProvider(
//	    opentelemetry.WithEndpoint("http://prometheus:9090"),
//	)
//	compiled.Invoke(ctx, messages, graph.WithMetrics(metricsProvider))
//
// In node RunFunc:
//
//	func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
//	    mp := metrics.FromContext(ctx)
//	    counter := mp.Counter("operations.count")
//	    counter.Add(ctx, 1, metrics.Attr{Key: "node", Value: "my_node"})
//	    // ...
//	}
func WithMetrics(metricsProvider metrics.Provider) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		opts.metricsProvider = metricsProvider
	}
}

// =============================================================================
// Checkpoint Helper Methods
// =============================================================================

// createCheckpoint builds a checkpoint from current graph state
func (cg *CompiledGraph) createCheckpoint(runID string, superstep int64, metadata map[string]any) *Checkpoint {
	if cg == nil || cg.stateManager == nil {
		return nil
	}

	// Get runtime state
	cg.runtimeMu.RLock()
	runtime := cg.runtime
	cg.runtimeMu.RUnlock()

	var completedNodes []string
	var pausedNodes []string
	if runtime != nil {
		completedNodes = runtime.completedNames()
		pausedNodes = runtime.pausedNames()
	}

	return &Checkpoint{
		RunID:          runID,
		Superstep:      superstep,
		Version:        cg.stateManager.Version(),
		Timestamp:      time.Now(),
		State:          cg.stateManager.GetAll(),
		Messages:       cg.stateManager.MessagesSnapshot(),
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
		Metadata:       metadata,
	}
}

// restoreCheckpoint applies a checkpoint to the current graph state
func (cg *CompiledGraph) restoreCheckpoint(checkpoint *Checkpoint) error {
	if cg == nil || checkpoint == nil {
		return nil
	}

	// Validate checkpoint version (detect corruption or sequence errors)
	if cg.stateManager != nil {
		currentVersion := cg.stateManager.Version()
		if checkpoint.Version > 0 && currentVersion > checkpoint.Version {
			return fmt.Errorf("checkpoint version mismatch: current state version %d is ahead of checkpoint version %d (possible concurrent modification or restore out of sequence)", currentVersion, checkpoint.Version)
		}
	}

	// Restore state
	if cg.stateManager != nil {
		cg.stateManager.ApplyUpdates(checkpoint.State, checkpoint.Messages)
		// Restore version from checkpoint
		if gs, ok := cg.stateManager.(*GraphState); ok {
			gs.setVersion(checkpoint.Version)
		}
	}

	// Restore runtime execution state
	cg.runtimeMu.Lock()
	cg.runtime = ensureExecutionState(cg.runtime)
	cg.runtime.setSuperstep(checkpoint.Superstep)

	// Restore completed nodes
	for _, nodeName := range checkpoint.CompletedNodes {
		cg.runtime.markCompleted(nodeName)
	}

	// Restore paused nodes
	for _, nodeName := range checkpoint.PausedNodes {
		cg.runtime.markPaused(nodeName)
	}
	cg.runtimeMu.Unlock()

	return nil
}
