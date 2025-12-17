package graph

import (
	"context"
	"reflect"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func TestRestoreCheckpointSharesStateMap(t *testing.T) {
	ctx := context.Background()
	shared := map[string]any{"foo": "bar"}
	runCfg := &runConfig{
		checkpoint: &checkpoint.Checkpoint{
			State:     shared,
			Committed: true,
		},
	}

	checkpointCfg := CheckpointConfig{}
	res, ok := restoreCheckpoint(ctx, checkpointCfg, runCfg, func(_ message.Message, _ error) bool { return true })
	if !ok {
		t.Fatal("restore aborted unexpectedly")
	}
	if res.State == nil {
		t.Fatal("expected state to be restored")
	}
	if reflect.ValueOf(res.State).Pointer() != reflect.ValueOf(shared).Pointer() {
		t.Fatalf("expected checkpoint map to be reused, got different pointer")
	}
}

func TestRestoreCheckpointCopyOnWriteOnMutation(t *testing.T) {
	// Note: State updates are now applied in initializeBSPState, not restoreCheckpoint.
	// This test verifies that restoreCheckpoint does NOT apply state updates (they're deferred).
	ctx := context.Background()
	shared := map[string]any{"foo": "bar"}
	updates := map[string]any{"foo": "baz", "new": 1}
	runCfg := &runConfig{
		checkpoint:   &checkpoint.Checkpoint{State: shared, Committed: true},
		stateUpdates: updates,
	}

	checkpointCfg := CheckpointConfig{}
	res, ok := restoreCheckpoint(ctx, checkpointCfg, runCfg, func(_ message.Message, _ error) bool { return true })
	if !ok {
		t.Fatal("restore aborted unexpectedly")
	}

	// With the new architecture, restoreCheckpoint does NOT apply state updates.
	// State updates are applied later in initializeBSPState using reducers.
	// So the state should still be the shared checkpoint state.
	if reflect.ValueOf(res.State).Pointer() != reflect.ValueOf(shared).Pointer() {
		t.Fatal("expected checkpoint state to be reused (updates are deferred)")
	}

	// State should still have original values (updates not applied yet)
	if got := res.State["foo"]; got != "bar" {
		t.Fatalf("expected foo=bar (original), got %v", got)
	}
}
