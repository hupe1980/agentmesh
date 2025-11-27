package graph_test

import (
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRunID(t *testing.T) {
	opts := &graph.RunOptions{}
	opt := graph.WithRunID("test-run-123")
	opt(opts)

	assert.Equal(t, "test-run-123", opts.RunID)
}

func TestWithRunID_NilOptions(t *testing.T) {
	opt := graph.WithRunID("test-run")
	// Should not panic
	opt(nil)
}

func TestWithCheckpointer(t *testing.T) {
	opts := &graph.RunOptions{}
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	opt := graph.WithCheckpointer(checkpointer)
	opt(opts)

	assert.Equal(t, checkpointer, opts.Checkpointer)
}

func TestWithCheckpointer_NilCheckpointer(t *testing.T) {
	opts := &graph.RunOptions{}
	opt := graph.WithCheckpointer(nil)
	opt(opts)

	assert.Nil(t, opts.Checkpointer)
}

func TestWithCheckpointOptions(t *testing.T) {
	opts := &graph.RunOptions{}
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	opt := graph.WithCheckpointOptions(
		checkpoint.WithCheckpointer(checkpointer),
		checkpoint.WithSaveInterval(5),
		checkpoint.WithAutoRestore(true),
	)
	opt(opts)

	assert.Equal(t, checkpointer, opts.Checkpointer)
	assert.Equal(t, 5, opts.CheckpointInterval)
	assert.True(t, opts.AutoRestore)
}

func TestWithResumeFromSuperstep(t *testing.T) {
	t.Run("resume_from_latest", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithResumeFromSuperstep(0)
		opt(opts)

		assert.Equal(t, int64(0), opts.ResumeFrom)
		assert.True(t, opts.AutoRestore)
	})

	t.Run("resume_from_specific", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithResumeFromSuperstep(5)
		opt(opts)

		assert.Equal(t, int64(5), opts.ResumeFrom)
		assert.True(t, opts.AutoRestore)
	})
}

func TestWithFailOnCheckpointError(t *testing.T) {
	t.Run("fail_enabled", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithFailOnCheckpointError(true)
		opt(opts)

		assert.True(t, opts.FailOnCheckpointErr)
	})

	t.Run("fail_disabled", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithFailOnCheckpointError(false)
		opt(opts)

		assert.False(t, opts.FailOnCheckpointErr)
	})
}

func TestWithCheckpointQueueSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{
			name:     "normal_size",
			size:     50,
			expected: 50,
		},
		{
			name:     "zero_size_synchronous",
			size:     0,
			expected: 0,
		},
		{
			name:     "minimal_buffering",
			size:     1,
			expected: 1,
		},
		{
			name:     "negative_size_becomes_zero",
			size:     -10,
			expected: 0,
		},
		{
			name:     "large_size",
			size:     1000,
			expected: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &graph.RunOptions{}
			opt := graph.WithCheckpointQueueSize(tt.size)
			opt(opts)

			assert.Equal(t, tt.expected, opts.CheckpointQueueSize)
		})
	}
}

func TestWithCheckpointStopTimeout(t *testing.T) {
	t.Run("standard_timeout", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithCheckpointStopTimeout(30 * time.Second)
		opt(opts)

		assert.Equal(t, 30*time.Second, opts.CheckpointStopTimeout)
	})

	t.Run("fast_shutdown", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithCheckpointStopTimeout(5 * time.Second)
		opt(opts)

		assert.Equal(t, 5*time.Second, opts.CheckpointStopTimeout)
	})

	t.Run("zero_timeout", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithCheckpointStopTimeout(0)
		opt(opts)

		assert.Equal(t, time.Duration(0), opts.CheckpointStopTimeout)
	})
}

func TestWithMaxConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
	}{
		{"low_concurrency", 1},
		{"default_concurrency", 4},
		{"high_concurrency", 16},
		{"very_high_concurrency", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &graph.RunOptions{}
			opt := graph.WithMaxConcurrency(tt.concurrency)
			opt(opts)

			assert.Equal(t, tt.concurrency, opts.MaxConcurrency)
		})
	}
}

func TestWithMaxIterations(t *testing.T) {
	tests := []struct {
		name       string
		iterations int
	}{
		{"minimal_iterations", 1},
		{"default_iterations", 100},
		{"many_iterations", 1000},
		{"unlimited", 0}, // Special case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &graph.RunOptions{}
			opt := graph.WithMaxIterations(tt.iterations)
			opt(opts)

			assert.Equal(t, tt.iterations, opts.MaxIterations)
		})
	}
}

func TestWithInitialSuperstep(t *testing.T) {
	tests := []struct {
		name      string
		superstep int64
	}{
		{"start_at_zero", 0},
		{"start_at_five", 5},
		{"start_at_large", 9999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &graph.RunOptions{}
			opt := graph.WithInitialSuperstep(tt.superstep)
			opt(opts)

			assert.Equal(t, tt.superstep, opts.ResumeFrom)
		})
	}
}

func TestWithCheckpoint(t *testing.T) {
	t.Run("valid_checkpoint", func(t *testing.T) {
		opts := &graph.RunOptions{}
		cp := &checkpoint.Checkpoint{
			RunID:     "test-run",
			Superstep: 5,
			State:     map[string]any{},
		}

		opt := graph.WithCheckpoint(cp)
		opt(opts)

		assert.Equal(t, cp, opts.Checkpoint)
	})

	t.Run("nil_checkpoint", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithCheckpoint(nil)
		opt(opts)

		assert.Nil(t, opts.Checkpoint)
	})

	t.Run("nil_options", func(t *testing.T) {
		cp := &checkpoint.Checkpoint{RunID: "test"}
		opt := graph.WithCheckpoint(cp)
		// Should not panic
		opt(nil)
	})
}

func TestWithResumeValue(t *testing.T) {
	t.Run("with_resume_data", func(t *testing.T) {
		opts := &graph.RunOptions{}
		resumeData := map[string]any{
			"approval": "APPROVED",
			"edited":   "new content",
			"score":    95,
		}

		opt := graph.WithResumeValue(resumeData)
		opt(opts)

		require.NotNil(t, opts.ResumeValue)
		assert.Equal(t, "APPROVED", opts.ResumeValue["approval"])
		assert.Equal(t, "new content", opts.ResumeValue["edited"])
		assert.Equal(t, 95, opts.ResumeValue["score"])
	})

	t.Run("empty_resume_data", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithResumeValue(map[string]any{})
		opt(opts)

		require.NotNil(t, opts.ResumeValue)
		assert.Empty(t, opts.ResumeValue)
	})

	t.Run("nil_resume_data", func(t *testing.T) {
		opts := &graph.RunOptions{}
		opt := graph.WithResumeValue(nil)
		opt(opts)

		assert.Nil(t, opts.ResumeValue)
	})
}

func TestRunOptions_ChainMultiple(t *testing.T) {
	opts := &graph.RunOptions{}

	// Apply multiple options in sequence
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	graph.WithRunID("chain-test")(opts)
	graph.WithCheckpointer(checkpointer)(opts)
	graph.WithMaxConcurrency(8)(opts)
	graph.WithMaxIterations(500)(opts)
	graph.WithFailOnCheckpointError(true)(opts)
	graph.WithCheckpointQueueSize(20)(opts)

	assert.Equal(t, "chain-test", opts.RunID)
	assert.Equal(t, checkpointer, opts.Checkpointer)
	assert.Equal(t, 8, opts.MaxConcurrency)
	assert.Equal(t, 500, opts.MaxIterations)
	assert.True(t, opts.FailOnCheckpointErr)
	assert.Equal(t, 20, opts.CheckpointQueueSize)
}

func TestRunOptions_OverrideValues(t *testing.T) {
	opts := &graph.RunOptions{}

	// Set initial value
	graph.WithMaxConcurrency(4)(opts)
	assert.Equal(t, 4, opts.MaxConcurrency)

	// Override with new value
	graph.WithMaxConcurrency(16)(opts)
	assert.Equal(t, 16, opts.MaxConcurrency)
}

func TestRunOptions_NilSafety(t *testing.T) {
	// All option functions should handle nil *RunOptions gracefully
	var opts *graph.RunOptions

	// These should not panic
	graph.WithRunID("test")(opts)
	graph.WithCheckpointer(nil)(opts)
	graph.WithMaxConcurrency(4)(opts)
	graph.WithMaxIterations(100)(opts)
	graph.WithFailOnCheckpointError(true)(opts)
	graph.WithCheckpointQueueSize(10)(opts)
	graph.WithCheckpointStopTimeout(30 * time.Second)(opts)
	graph.WithInitialSuperstep(0)(opts)
	graph.WithResumeFromSuperstep(0)(opts)
	graph.WithCheckpoint(nil)(opts)
	graph.WithResumeValue(nil)(opts)
}

func TestRunOptions_ComplexScenario(t *testing.T) {
	// Simulate a complex human-in-the-loop workflow
	opts := &graph.RunOptions{}
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Initial execution settings
	graph.WithRunID("user-123-session-abc")(opts)
	graph.WithCheckpointer(checkpointer)(opts)
	graph.WithCheckpointOptions(
		checkpoint.WithSaveInterval(1),
		checkpoint.WithAutoRestore(true),
	)(opts)
	graph.WithMaxConcurrency(4)(opts)
	graph.WithMaxIterations(100)(opts)
	graph.WithFailOnCheckpointError(true)(opts)

	// Later, resume with human input
	cp := &checkpoint.Checkpoint{
		RunID:     "user-123-session-abc",
		Superstep: 3,
		State:     map[string]any{},
	}

	graph.WithCheckpoint(cp)(opts)
	graph.WithResumeValue(map[string]any{
		"approval":   "APPROVED",
		"confidence": 0.95,
		"notes":      "Looks good!",
	})(opts)

	// Verify all settings
	assert.Equal(t, "user-123-session-abc", opts.RunID)
	assert.NotNil(t, opts.Checkpointer)
	assert.Equal(t, 1, opts.CheckpointInterval)
	assert.True(t, opts.AutoRestore)
	assert.Equal(t, 4, opts.MaxConcurrency)
	assert.Equal(t, 100, opts.MaxIterations)
	assert.True(t, opts.FailOnCheckpointErr)
	assert.Equal(t, cp, opts.Checkpoint)
	assert.Equal(t, "APPROVED", opts.ResumeValue["approval"])
	assert.Equal(t, 0.95, opts.ResumeValue["confidence"])
	assert.Equal(t, "Looks good!", opts.ResumeValue["notes"])
}
