package graph

import (
	"context"
	"reflect"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
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
	res, ok := restoreCheckpoint(ctx, checkpointCfg, runCfg, func(_ any, _ error) bool { return true })
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
	ctx := context.Background()
	shared := map[string]any{"foo": "bar"}
	updates := map[string]any{"foo": "baz", "new": 1}
	runCfg := &runConfig{
		checkpoint:   &checkpoint.Checkpoint{State: shared, Committed: true},
		stateUpdates: updates,
	}

	checkpointCfg := CheckpointConfig{}
	res, ok := restoreCheckpoint(ctx, checkpointCfg, runCfg, func(_ any, _ error) bool { return true })
	if !ok {
		t.Fatal("restore aborted unexpectedly")
	}
	if reflect.ValueOf(res.State).Pointer() == reflect.ValueOf(shared).Pointer() {
		t.Fatal("expected state map to be copied when applying updates")
	}
	if got := res.State["foo"]; got != "baz" {
		t.Fatalf("expected foo=baz, got %v", got)
	}
	if _, ok := res.State["new"]; !ok {
		t.Fatal("expected overlay value to be present")
	}
	if shared["foo"] != "bar" {
		t.Fatal("checkpoint state should remain untouched")
	}
}
