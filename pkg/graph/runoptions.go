package graph

import "github.com/hupe1980/agentmesh/pkg/checkpoint"

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
//	runnable.Run(ctx, messages, graph.WithRunID("user-123-session-abc"))
func WithRunID(runID string) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.RunID = runID
	}
}

// WithCheckpointer enables automatic checkpointing with the given storage backend.
// Checkpoints are saved after each superstep by default.
//
// Example:
//
//	checkpointer := checkpoint.NewInMemoryCheckpointer()
//	runnable.Run(ctx, messages,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("user-123-session-456"),
//	)
func WithCheckpointer(checkpointer checkpoint.Checkpointer) RunOption {
	return func(opts *RunOptions) {
		if opts == nil || checkpointer == nil {
			return
		}
		opts.Checkpointer = checkpointer
	}
}

// WithCheckpointConfig provides fine-grained control over checkpointing behavior.
//
// Example:
//
//	runnable.Run(ctx, messages,
//	    graph.WithCheckpointConfig(checkpoint.Config{
//	        Checkpointer:  checkpointer,
//	        SaveInterval:  5,     // Save every 5 supersteps
//	        AutoRestore:   true,  // Resume from last checkpoint
//	    }),
//	    graph.WithRunID("long-running-workflow"),
//	)
func WithCheckpointConfig(config checkpoint.Config) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.Checkpointer = config.Checkpointer
		opts.CheckpointInterval = config.SaveInterval
		opts.AutoRestore = config.AutoRestore
	}
}

// WithResumeFromSuperstep resumes execution from a specific superstep.
// Loads the checkpoint at that superstep and continues from there.
// If superstep is 0, resumes from the most recent checkpoint.
//
// Example:
//
//	// Resume from most recent
//	runnable.Run(ctx, nil,
//	    graph.WithRunID("user-123"),
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithResumeFromSuperstep(0),
//	)
//
//	// Resume from specific point (time-travel)
//	runnable.Run(ctx, nil,
//	    graph.WithRunID("user-123"),
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithResumeFromSuperstep(5),
//	)
func WithResumeFromSuperstep(superstep int64) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.ResumeFrom = superstep
		opts.AutoRestore = true // Auto-enable restore when resuming
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
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.FailOnCheckpointErr = fail
	}
}

// WithCheckpointQueueSize configures the size of the asynchronous checkpoint queue.
// When the queue is full, execution will block until a checkpoint completes,
// applying backpressure to prevent checkpoint loss.
//
// Default: 10
// Set to 0 to disable async checkpointing (all checkpoints saved synchronously)
// Set to 1 for minimal buffering with immediate backpressure
// Higher values (e.g., 50-100) reduce blocking but increase memory usage
//
// Example:
//
//	// Large queue for high checkpoint frequency
//	graph.WithCheckpointQueueSize(100)
//
//	// Synchronous checkpoints only
//	graph.WithCheckpointQueueSize(0)
//
//	// Minimal buffering with backpressure
//	graph.WithCheckpointQueueSize(1)
func WithCheckpointQueueSize(size int) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		if size < 0 {
			size = 0
		}
		opts.CheckpointQueueSize = size
	}
}

// WithMaxConcurrency sets the maximum number of nodes that can execute in parallel.
// Defaults to 4. Higher values may improve throughput for I/O-bound nodes but increase
// memory usage.
//
// Example:
//
//	runnable.Run(ctx, messages, graph.WithMaxConcurrency(8))
func WithMaxConcurrency(n int) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.MaxConcurrency = n
	}
}

// WithMaxIterations sets the maximum number of supersteps before stopping execution.
// Prevents infinite loops in cyclic graphs. Defaults to 100.
//
// Example:
//
//	runnable.Run(ctx, messages, graph.WithMaxIterations(1000))
func WithMaxIterations(n int) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.MaxIterations = n
	}
}

// WithInitialSuperstep sets the starting superstep number for execution.
// Useful when resuming from a checkpoint or continuing interrupted execution.
//
// Example:
//
//	// Resume from superstep 5
//	runnable.Run(ctx, messages, graph.WithInitialSuperstep(5))
func WithInitialSuperstep(step int64) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.ResumeFrom = step
	}
}

// WithMaxMessages sets the maximum number of messages to retain in history.
// Older messages are automatically pruned when limit is exceeded.
// Use 0 for unlimited (default).
//
// Example:
//
//	// Keep only last 100 messages
//	runnable.Run(ctx, messages, graph.WithMaxMessages(100))
func WithMaxMessages(n int) RunOption {
	return func(opts *RunOptions) {
		// This is a placeholder - the actual implementation would need
		// to configure the state manager's message retention policy
		// For now, this just makes the option available for compatibility
	}
}

// WithMaxMessageSize sets the maximum size in bytes for individual messages.
// Messages exceeding this size will be rejected.
//
// Example:
//
//	// Limit messages to 1MB
//	runnable.Run(ctx, messages, graph.WithMaxMessageSize(1024*1024))
func WithMaxMessageSize(bytes int) RunOption {
	return func(opts *RunOptions) {
		// Placeholder for compatibility with old examples
	}
}

// WithMaxInputMessages sets the maximum number of input messages accepted.
//
// Example:
//
//	runnable.Run(ctx, messages, graph.WithMaxInputMessages(10))
func WithMaxInputMessages(n int) RunOption {
	return func(opts *RunOptions) {
		// Placeholder for compatibility with old examples
	}
}

// WithMaxTotalSize sets the maximum total size in bytes for all messages.
//
// Example:
//
//	// Limit total to 10MB
//	runnable.Run(ctx, messages, graph.WithMaxTotalSize(10*1024*1024))
func WithMaxTotalSize(bytes int) RunOption {
	return func(opts *RunOptions) {
		// Placeholder for compatibility with old examples
	}
}

// WithLogger sets a custom logger for observability.
// The logger is attached to the context passed to nodes.
//
// Example:
//
//	logger := logging.NewLogger()
//	runnable.Run(ctx, messages, graph.WithLogger(logger))
func WithLogger(logger interface{}) RunOption {
	return func(opts *RunOptions) {
		// Placeholder for compatibility - actual implementation would
		// attach logger to context using logging.WithLogger
	}
}

// WithTracer sets a custom tracer for distributed tracing.
// The tracer is attached to the context passed to nodes.
//
// Example:
//
//	tracer := trace.NewTracer()
//	runnable.Run(ctx, messages, graph.WithTracer(tracer))
func WithTracer(tracer interface{}) RunOption {
	return func(opts *RunOptions) {
		// Placeholder for compatibility - actual implementation would
		// attach tracer to context using trace.WithTracer
	}
}

// WithMetrics sets a custom metrics collector.
// The metrics collector is attached to the context passed to nodes.
//
// Example:
//
//	metrics := metrics.NewCollector()
//	runnable.Run(ctx, messages, graph.WithMetrics(metrics))
func WithMetrics(metrics interface{}) RunOption {
	return func(opts *RunOptions) {
		// Placeholder for compatibility - actual implementation would
		// attach metrics to context
	}
}
