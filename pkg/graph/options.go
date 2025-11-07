package graph

import (
	"runtime"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Aggregator defines a global reduction applied across vertices each superstep.
// Implementations combine values from multiple nodes into a single aggregate value.
// See aggregators.go for built-in implementations (SumAggregator, MaxAggregator, etc.).
type Aggregator interface {
	// Zero returns the identity/initial value for the aggregation.
	Zero() any
	// Aggregate combines the current aggregate value with a new contribution.
	Aggregate(current, value any) any
}

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
	maxConcurrency     int
	initialSuperstep   int64
	maxIterations      int
	maxMessages        int // Maximum number of messages to retain (0 = unlimited)
	aggregators        map[string]Aggregator
	combiner           Combiner
	checkpointer       Checkpointer // Checkpoint storage backend
	checkpointInterval int          // Save every N supersteps (0 = every superstep)
	autoRestore        bool         // Automatically restore from last checkpoint
	runID              string       // Unique identifier for this execution run
	resume             bool         // Resume from checkpoint
	resumeFrom         int64        // Superstep to resume from (0 = most recent)
}

type RunOption func(*runOptions)

func defaultRunOptions() runOptions {
	return runOptions{
		maxConcurrency: runtime.NumCPU(),
		maxMessages:    0, // Unlimited by default
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
//	compiled.Invoke(ctx, messages, graph.WithAggregators(map[string]Aggregator{
//	    "total_cost": &SumAggregator{},
//	    "max_priority": &MaxAggregator{},
//	}))
func WithAggregators(aggregators map[string]Aggregator) RunOption {
	return func(opts *runOptions) {
		if opts == nil {
			return
		}
		if len(aggregators) == 0 {
			opts.aggregators = nil
			return
		}
		aggCopy := make(map[string]Aggregator, len(aggregators))
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

	// Restore state
	if cg.stateManager != nil {
		cg.stateManager.ApplyUpdates(checkpoint.State, checkpoint.Messages)
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
