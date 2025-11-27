package viz

import (
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertCheckpointToMetadata(t *testing.T) {
	now := time.Now()

	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 5,
		Version:   10,
		Timestamp: now,
		Committed: true,
		State: map[string]any{
			"messages": []string{"msg1", "msg2"},
			"counter":  42,
		},
		CompletedNodes: []string{"node1", "node2"},
		PausedNodes:    []string{"node3"},
		PendingWrites: []checkpoint.PendingWrite{
			{NodeName: "node1", Channel: "messages", Value: "test"},
		},
		Metadata: map[string]any{
			"input_tokens":  100,
			"output_tokens": 50,
			"total_tokens":  150,
			"cost_usd":      0.05,
		},
	}

	allCheckpoints := []*checkpoint.Checkpoint{
		{Superstep: 1},
		{Superstep: 3},
		cp, // Current at index 2
		{Superstep: 7},
	}

	metadata := ConvertCheckpointToMetadata(cp, allCheckpoints)

	// Verify basic info
	assert.Equal(t, "test-run", metadata.RunID)
	assert.Equal(t, int64(5), metadata.Superstep)
	assert.Equal(t, uint64(10), metadata.Version)
	assert.Equal(t, now, metadata.Timestamp)
	assert.True(t, metadata.Committed)

	// Verify counts
	assert.Equal(t, 2, metadata.CompletedCount)
	assert.Equal(t, 1, metadata.PausedCount)
	assert.Equal(t, 1, metadata.PendingCount)

	// Verify nodes
	assert.Equal(t, []string{"node1", "node2"}, metadata.CompletedNodes)
	assert.Equal(t, []string{"node3"}, metadata.PausedNodes)

	// Verify state keys
	assert.Len(t, metadata.StateKeys, 2)
	assert.Contains(t, metadata.StateKeys, "messages")
	assert.Contains(t, metadata.StateKeys, "counter")

	// Verify token usage
	assert.Equal(t, 100, metadata.InputTokens)
	assert.Equal(t, 50, metadata.OutputTokens)
	assert.Equal(t, 150, metadata.TotalTokens)
	assert.Equal(t, 0.05, metadata.EstCostUSD)

	// Verify navigation
	assert.True(t, metadata.HasPrevious)
	assert.True(t, metadata.HasNext)
	assert.Equal(t, int64(3), metadata.PrevStep)
	assert.Equal(t, int64(7), metadata.NextStep)
}

func TestConvertCheckpointToMetadata_FirstCheckpoint(t *testing.T) {
	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		Committed: false,
		State:     map[string]any{},
	}

	allCheckpoints := []*checkpoint.Checkpoint{
		cp, // First checkpoint
		{Superstep: 2},
		{Superstep: 3},
	}

	metadata := ConvertCheckpointToMetadata(cp, allCheckpoints)

	// First checkpoint should not have previous
	assert.False(t, metadata.HasPrevious)
	assert.True(t, metadata.HasNext)
	assert.Equal(t, int64(2), metadata.NextStep)
}

func TestConvertCheckpointToMetadata_LastCheckpoint(t *testing.T) {
	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 5,
		Version:   5,
		Timestamp: time.Now(),
		Committed: true,
		State:     map[string]any{},
	}

	allCheckpoints := []*checkpoint.Checkpoint{
		{Superstep: 1},
		{Superstep: 3},
		cp, // Last checkpoint
	}

	metadata := ConvertCheckpointToMetadata(cp, allCheckpoints)

	// Last checkpoint should not have next
	assert.True(t, metadata.HasPrevious)
	assert.False(t, metadata.HasNext)
	assert.Equal(t, int64(3), metadata.PrevStep)
}

func TestConvertCheckpointToMetadata_NoMetadata(t *testing.T) {
	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		Committed: true,
		State:     map[string]any{},
		Metadata:  nil, // No metadata
	}

	metadata := ConvertCheckpointToMetadata(cp, []*checkpoint.Checkpoint{cp})

	// Should handle nil metadata gracefully
	assert.Equal(t, 0, metadata.InputTokens)
	assert.Equal(t, 0, metadata.OutputTokens)
	assert.Equal(t, 0, metadata.TotalTokens)
	assert.Equal(t, 0.0, metadata.EstCostUSD)
}

func TestConvertCheckpointToMetadata_EmptyCheckpointList(t *testing.T) {
	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		Committed: true,
		State:     map[string]any{},
	}

	metadata := ConvertCheckpointToMetadata(cp, []*checkpoint.Checkpoint{})

	// Should handle empty checkpoint list
	assert.False(t, metadata.HasPrevious)
	assert.False(t, metadata.HasNext)
}

func TestPendingWriteConversion(t *testing.T) {
	now := time.Now()

	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Version:   1,
		Timestamp: now,
		Committed: false,
		State:     map[string]any{},
		PendingWrites: []checkpoint.PendingWrite{
			{
				NodeName:  "node1",
				Channel:   "messages",
				Value:     "hello",
				Timestamp: now,
			},
			{
				NodeName:  "node2",
				Channel:   "state",
				Value:     map[string]any{"key": "value"},
				Timestamp: now,
			},
		},
	}

	metadata := ConvertCheckpointToMetadata(cp, []*checkpoint.Checkpoint{cp})

	require.Len(t, metadata.PendingWrites, 2)

	// Verify first pending write
	assert.Equal(t, "node1", metadata.PendingWrites[0].NodeName)
	assert.Equal(t, "messages", metadata.PendingWrites[0].Channel)
	assert.Equal(t, "hello", metadata.PendingWrites[0].Value)
	assert.Equal(t, now, metadata.PendingWrites[0].Timestamp)

	// Verify second pending write
	assert.Equal(t, "node2", metadata.PendingWrites[1].NodeName)
	assert.Equal(t, "state", metadata.PendingWrites[1].Channel)
	assert.Equal(t, now, metadata.PendingWrites[1].Timestamp)
}

func TestStringDiff(t *testing.T) {
	t.Run("elements in a not in b", func(t *testing.T) {
		a := []string{"apple", "banana", "cherry"}
		b := []string{"banana"}

		diff := stringDiff(a, b)
		assert.Len(t, diff, 2)
		assert.Contains(t, diff, "apple")
		assert.Contains(t, diff, "cherry")
	})

	t.Run("no difference", func(t *testing.T) {
		a := []string{"apple", "banana"}
		b := []string{"apple", "banana"}

		diff := stringDiff(a, b)
		assert.Empty(t, diff)
	})

	t.Run("all elements different", func(t *testing.T) {
		a := []string{"apple", "banana"}
		b := []string{"cherry", "date"}

		diff := stringDiff(a, b)
		assert.Len(t, diff, 2)
		assert.Contains(t, diff, "apple")
		assert.Contains(t, diff, "banana")
	})

	t.Run("empty slices", func(t *testing.T) {
		a := []string{}
		b := []string{"apple"}

		diff := stringDiff(a, b)
		assert.Empty(t, diff)
	})

	t.Run("b is empty", func(t *testing.T) {
		a := []string{"apple", "banana"}
		b := []string{}

		diff := stringDiff(a, b)
		assert.Len(t, diff, 2)
		assert.Contains(t, diff, "apple")
		assert.Contains(t, diff, "banana")
	})
}

func TestCheckpointDiffResponse_Structure(t *testing.T) {
	diff := CheckpointDiffResponse{
		FromSuperstep: 1,
		ToSuperstep:   3,
		StateDiffs: []StateDiff{
			{Type: DiffTypeAdded, Key: "new_key"},
			{Type: DiffTypeModified, Key: "changed_key"},
			{Type: DiffTypeRemoved, Key: "old_key"},
		},
		Summary: DiffSummary{
			AddedKeys:     1,
			RemovedKeys:   1,
			ModifiedKeys:  1,
			NodesAdded:    []string{"node2"},
			NodesRemoved:  []string{},
			WritesApplied: 2,
		},
	}

	assert.Equal(t, int64(1), diff.FromSuperstep)
	assert.Equal(t, int64(3), diff.ToSuperstep)
	assert.Len(t, diff.StateDiffs, 3)
	assert.Equal(t, 1, diff.Summary.AddedKeys)
	assert.Equal(t, 1, diff.Summary.RemovedKeys)
	assert.Equal(t, 1, diff.Summary.ModifiedKeys)
	assert.Equal(t, 2, diff.Summary.WritesApplied)
}

func TestEnhancedCheckpointMetadata_Complete(t *testing.T) {
	metadata := EnhancedCheckpointMetadata{
		RunID:          "test-run",
		Superstep:      5,
		Version:        10,
		Timestamp:      time.Now(),
		Committed:      true,
		Duration:       1 * time.Second,
		TotalDuration:  5 * time.Second,
		MemoryUsageKB:  1024,
		NodeCount:      5,
		CompletedCount: 3,
		PausedCount:    1,
		PendingCount:   2,
		CompletedNodes: []string{"node1", "node2", "node3"},
		PausedNodes:    []string{"node4"},
		ActiveNodes:    []string{"node5"},
		StateKeys:      []string{"messages", "counter"},
		StateSize:      2048,
		MessageCount:   10,
		InputTokens:    100,
		OutputTokens:   50,
		TotalTokens:    150,
		EstCostUSD:     0.05,
		HasPrevious:    true,
		HasNext:        true,
		PrevStep:       3,
		NextStep:       7,
	}

	// Verify all fields are set
	assert.Equal(t, "test-run", metadata.RunID)
	assert.Equal(t, int64(5), metadata.Superstep)
	assert.Equal(t, uint64(10), metadata.Version)
	assert.True(t, metadata.Committed)
	assert.Equal(t, 1*time.Second, metadata.Duration)
	assert.Equal(t, 5*time.Second, metadata.TotalDuration)
	assert.Equal(t, uint64(1024), metadata.MemoryUsageKB)
	assert.Equal(t, 5, metadata.NodeCount)
	assert.Equal(t, 3, metadata.CompletedCount)
	assert.Equal(t, 1, metadata.PausedCount)
	assert.Equal(t, 2, metadata.PendingCount)
	assert.Len(t, metadata.CompletedNodes, 3)
	assert.Len(t, metadata.PausedNodes, 1)
	assert.Len(t, metadata.ActiveNodes, 1)
	assert.Len(t, metadata.StateKeys, 2)
	assert.Equal(t, 2048, metadata.StateSize)
	assert.Equal(t, 10, metadata.MessageCount)
	assert.Equal(t, 100, metadata.InputTokens)
	assert.Equal(t, 50, metadata.OutputTokens)
	assert.Equal(t, 150, metadata.TotalTokens)
	assert.Equal(t, 0.05, metadata.EstCostUSD)
	assert.True(t, metadata.HasPrevious)
	assert.True(t, metadata.HasNext)
	assert.Equal(t, int64(3), metadata.PrevStep)
	assert.Equal(t, int64(7), metadata.NextStep)
}
