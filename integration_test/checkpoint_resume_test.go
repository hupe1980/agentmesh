package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckpointResume_BasicSaveLoad tests basic checkpoint save and load.
func TestCheckpointResume_BasicSaveLoad(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-1"

	// Create and save a checkpoint
	cp := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State: map[string]any{
			"result": "test-value",
		},
		CompletedNodes: []string{"node1"},
	}

	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	// Load the checkpoint
	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, runID, loaded.RunID)
	assert.Equal(t, int64(1), loaded.Superstep)
	assert.Equal(t, "test-value", loaded.State["result"])
	assert.Contains(t, loaded.CompletedNodes, "node1")
}

// TestCheckpointResume_MultipleCheckpoints tests saving multiple checkpoints.
func TestCheckpointResume_MultipleCheckpoints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-multi"

	// Save multiple checkpoints
	for i := int64(1); i <= 5; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: i,
			Version:   uint64(i),
			Timestamp: time.Now(),
			State: map[string]any{
				"step": i,
			},
		}
		err := checkpointer.Save(ctx, cp)
		require.NoError(t, err)
	}

	// Load latest should return step 5
	latest, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), latest.Superstep)
}

// TestCheckpointResume_LoadAtSuperstep tests loading at a specific superstep.
func TestCheckpointResume_LoadAtSuperstep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-superstep"

	// Save checkpoints at different supersteps
	for i := int64(1); i <= 3; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: i,
			Version:   uint64(i),
			Timestamp: time.Now(),
			State: map[string]any{
				"step": i,
			},
		}
		err := checkpointer.Save(ctx, cp)
		require.NoError(t, err)
	}

	// Load at superstep 2
	cp, err := checkpointer.LoadAtSuperstep(ctx, runID, 2)
	require.NoError(t, err)
	require.NotNil(t, cp)
	assert.Equal(t, int64(2), cp.Superstep)
}

// TestCheckpointResume_WithSigning tests checkpoint integrity with signing.
func TestCheckpointResume_WithSigning(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	signingKey := []byte("test-signing-key-for-checkpoint-integrity")
	checkpointer := checkpoint.NewInMemoryCheckpointer(checkpoint.WithSigning(signingKey))
	runID := "test-run-signed"

	cp := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State: map[string]any{
			"data": "signed-data",
		},
	}

	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	// Load should verify signature
	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "signed-data", loaded.State["data"])
}

// TestCheckpointResume_PausedNodes tests checkpoint with paused nodes.
func TestCheckpointResume_PausedNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-paused"

	cp := &checkpoint.Checkpoint{
		RunID:       runID,
		Superstep:   1,
		Version:     1,
		Timestamp:   time.Now(),
		State:       map[string]any{},
		PausedNodes: []string{"human_approval", "review"},
	}

	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Contains(t, loaded.PausedNodes, "human_approval")
	assert.Contains(t, loaded.PausedNodes, "review")
}

// TestCheckpointResume_PendingWrites tests checkpoint with pending writes.
func TestCheckpointResume_PendingWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-pending"

	cp := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State:     map[string]any{},
		PendingWrites: []checkpoint.PendingWrite{
			{
				NodeName:  "processor",
				Channel:   "result",
				Value:     "pending-value",
				Timestamp: time.Now(),
			},
		},
		Committed: false,
	}

	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, loaded.PendingWrites, 1)
	assert.Equal(t, "processor", loaded.PendingWrites[0].NodeName)
	assert.False(t, loaded.Committed)
}

// TestCheckpointResume_MessageState tests checkpoint with message state.
func TestCheckpointResume_MessageState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-messages"

	messages := []message.Message{
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there!"),
	}

	cp := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State: map[string]any{
			"messages": messages,
		},
	}

	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	loadedMessages, ok := loaded.State["messages"].([]message.Message)
	require.True(t, ok)
	require.Len(t, loadedMessages, 2)
}

// TestCheckpointResume_Metadata tests checkpoint with custom metadata.
func TestCheckpointResume_Metadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-metadata"

	cp := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State:     map[string]any{},
		Metadata: map[string]any{
			"user_id":     "user-123",
			"session_id":  "session-456",
			"custom_data": map[string]string{"key": "value"},
		},
	}

	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "user-123", loaded.Metadata["user_id"])
	assert.Equal(t, "session-456", loaded.Metadata["session_id"])
}

// TestCheckpointResume_NonExistentRun tests loading a non-existent run.
func TestCheckpointResume_NonExistentRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	loaded, err := checkpointer.Load(ctx, "non-existent-run")
	require.NoError(t, err)
	assert.Nil(t, loaded) // Should return nil, not error
}

// TestCheckpointResume_GraphWithCheckpointer tests graph execution with checkpointer.
func TestCheckpointResume_GraphWithCheckpointer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	var finalResult string

	g := graph.New(ResultKey)

	g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		input := graph.Get(scope, ResultKey)
		finalResult = input + "_processed"
		scope.Stream(message.NewAIMessageFromText(finalResult))
		return graph.Set(ResultKey, finalResult).End()
	}, graph.END)

	g.Start("process")

	// Set checkpointer on graph before building
	g.WithCheckpointer(checkpointer, "checkpointed-run")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Run with initial value
	for _, err := range compiled.Run(ctx, nil, graph.WithInitialValue(ResultKey, "test")) {
		require.NoError(t, err)
	}

	assert.Equal(t, "test_processed", finalResult)

	// Verify checkpoint was saved
	cp, err := checkpointer.Load(ctx, "checkpointed-run")
	require.NoError(t, err)
	require.NotNil(t, cp)
}

// TestCheckpointResume_Delete tests checkpoint deletion.
func TestCheckpointResume_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "test-run-delete"

	// Save a checkpoint
	cp := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State:     map[string]any{},
	}
	err := checkpointer.Save(ctx, cp)
	require.NoError(t, err)

	// Verify it exists
	loaded, err := checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Delete
	err = checkpointer.Delete(ctx, runID)
	require.NoError(t, err)

	// Verify it's gone
	loaded, err = checkpointer.Load(ctx, runID)
	require.NoError(t, err)
	assert.Nil(t, loaded)
}
