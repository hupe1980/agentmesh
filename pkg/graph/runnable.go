package graph

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Runnable represents any executable component that can process input
// and stream results.
type Runnable[I, O any] interface {
	Run(ctx context.Context, input I, opts ...RunOption) iter.Seq2[O, error]
}

// RunOption configures graph execution behavior.
type RunOption func(*RunOptions)

// RunOptions holds runtime execution configuration.
type RunOptions struct {
	MaxIterations       int
	MaxConcurrency      int
	RunID               string
	Checkpointer        checkpoint.Checkpointer
	CheckpointInterval  int
	CheckpointQueueSize int // Size of async checkpoint queue (0 = synchronous only)
	AutoRestore         bool
	ResumeFrom          int64
	FailOnCheckpointErr bool
}

func defaultRunOptions() RunOptions {
	return RunOptions{
		MaxIterations:       100,
		MaxConcurrency:      4,
		CheckpointInterval:  1,  // Save every superstep by default
		CheckpointQueueSize: 10, // Buffer up to 10 checkpoints
		AutoRestore:         false,
		FailOnCheckpointErr: false,
	}
}

// ApplyOptions applies a slice of RunOption to RunOptions.
func ApplyOptions(opts ...RunOption) RunOptions {
	ro := defaultRunOptions()
	for _, opt := range opts {
		opt(&ro)
	}
	return ro
}
