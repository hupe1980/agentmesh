package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func BenchmarkRestoreCheckpoint10KKeys(b *testing.B) {
	chkpt := buildLargeCheckpoint(10_000)
	checkpointCfg := CheckpointConfig{}
	runCfg := &runConfig{checkpoint: chkpt}
	ctx := context.Background()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res, ok := restoreCheckpoint(ctx, checkpointCfg, runCfg, func(_ message.Message, _ error) bool { return true })
		if !ok {
			b.Fatal("restore aborted unexpectedly")
		}
		if len(res.State) != len(chkpt.State) {
			b.Fatalf("state size mismatch: got %d want %d", len(res.State), len(chkpt.State))
		}

		state := NewBSPState(res.State, NewKeyRegistry())
		state.ApplyPendingWrites(res.PendingWrites)

		// Touch a hot key to ensure pending writes landed.
		if got, ok := state.GetCommitted("hot_key"); !ok || got != "pending" {
			b.Fatalf("pending writes missing: %v", got)
		}
	}
}

func buildLargeCheckpoint(size int) *checkpoint.Checkpoint {
	state := make(map[string]any, size)
	for i := 0; i < size; i++ {
		state[fmt.Sprintf("key_%d", i)] = i
	}
	// Pending write exercises recovery path.
	pending := []checkpoint.PendingWrite{
		{NodeName: "resume", Channel: "hot_key", Value: "pending"},
	}

	return &checkpoint.Checkpoint{
		State:         state,
		PendingWrites: pending,
		Committed:     false,
	}
}
