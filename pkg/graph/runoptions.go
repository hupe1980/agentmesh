package graph

import (
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

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

// WithCheckpointOptions provides fine-grained control over checkpointing behavior.
//
// Example:
//
//	runnable.Run(ctx, messages,
//	    graph.WithCheckpointOptions(
//	        checkpoint.WithCheckpointer(checkpointer),
//	        checkpoint.WithSaveInterval(5),     // Save every 5 supersteps
//	        checkpoint.WithAutoRestore(true),   // Resume from last checkpoint
//	    ),
//	    graph.WithRunID("long-running-workflow"),
//	)
func WithCheckpointOptions(opts ...checkpoint.Option) RunOption {
	return func(runOpts *RunOptions) {
		if runOpts == nil {
			return
		}
		checkpointer, interval, autoRestore := checkpoint.ApplyOptions(opts)
		if checkpointer != nil {
			runOpts.Checkpointer = checkpointer
		}
		if interval > 0 {
			runOpts.CheckpointInterval = interval
		}
		runOpts.AutoRestore = autoRestore
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

// WithCheckpointStopTimeout sets the maximum time to wait for the checkpoint worker
// to finish processing queued checkpoints during shutdown. If the timeout is exceeded,
// an error is returned but execution continues.
//
// Default: 30 seconds
//
// Use cases:
//   - Set higher (e.g., 60s) for slow checkpoint storage (network, S3)
//   - Set lower (e.g., 5s) for fast local storage or when immediate shutdown is critical
//   - Set to 0 to wait indefinitely (not recommended)
//
// Example:
//
//	// Wait up to 60 seconds for checkpoints to complete
//	graph.WithCheckpointStopTimeout(60 * time.Second)
//
//	// Fast shutdown with 5 second timeout
//	graph.WithCheckpointStopTimeout(5 * time.Second)
func WithCheckpointStopTimeout(timeout time.Duration) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.CheckpointStopTimeout = timeout
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

// WithCheckpoint resumes execution from a saved checkpoint.
// The checkpoint's state is restored and any pending writes are applied
// before continuing execution. Typically used with WithResumeValue for
// human-in-the-loop workflows.
//
// Example:
//
//	// Resume from a paused checkpoint
//	checkpoint, _ := checkpointer.Load(ctx, runID)
//	compiled.Run(ctx, input,
//	    graph.WithCheckpoint(checkpoint),
//	    graph.WithResumeValue(map[string]any{
//	        "approval": "APPROVED",
//	    }),
//	)
func WithCheckpoint(cp *checkpoint.Checkpoint) RunOption {
	return func(opts *RunOptions) {
		if opts == nil || cp == nil {
			return
		}
		opts.Checkpoint = cp
	}
}

// WithResumeValue injects values into the execution context that nodes
// can access via ResumeValueFromContext(). This enables human-in-the-loop
// workflows where execution is paused for review and resumed with external input.
//
// Use cases:
//   - Human approval/rejection of AI actions
//   - Human edits to AI-generated content
//   - Injection of test values for debugging
//   - A/B testing with different parameters
//
// Example:
//
//	// Resume with human approval
//	compiled.Run(ctx, input,
//	    graph.WithCheckpoint(checkpoint),
//	    graph.WithResumeValue(map[string]any{
//	        "approval": "APPROVED",
//	        "edited_output": "Human-edited content...",
//	        "reason": "Looks good!",
//	    }),
//	)
//
//	// In node:
//	func (n *MyNode) Execute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
//	    if resume := graph.ResumeValueFromContext(ctx); resume != nil {
//	        if resume["approval"] == "APPROVED" {
//	            // Use human-approved path
//	        }
//	    }
//	    // Normal execution
//	}
func WithResumeValue(value map[string]any) RunOption {
	return func(opts *RunOptions) {
		if opts == nil {
			return
		}
		opts.ResumeValue = value
	}
}
