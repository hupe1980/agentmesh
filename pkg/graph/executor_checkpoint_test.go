package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// TestLoadAutoRestoredCheckpoint_Success tests successful auto-restore from checkpointer.
func TestLoadAutoRestoredCheckpoint_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	savedState := map[string]any{"restored": true}

	mockCheckpointer := &mockCheckpointer{
		loadFunc: func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
			return &checkpoint.Checkpoint{
				State:     savedState,
				Committed: true,
			}, nil
		},
	}

	cfg := &ExecutorConfig[any, string]{
		RunID:        "test-run",
		Checkpointer: mockCheckpointer,
	}

	runCfg := &runConfig{
		autoRestore: true,
	}

	result := &checkpointRestoreResult{}
	yield := func(_ string, _ error) bool { return true }

	ok := loadAutoRestoredCheckpoint(ctx, cfg, runCfg, result, yield)

	if !ok {
		t.Fatal("expected loadAutoRestoredCheckpoint to succeed")
	}
	if result.State == nil {
		t.Fatal("expected state to be populated from checkpoint")
	}
	if !reflect.DeepEqual(result.State, savedState) {
		t.Errorf("expected state %v, got %v", savedState, result.State)
	}
}

// TestLoadAutoRestoredCheckpoint_ErrorWithFailFast tests error handling with failOnCheckpointErr=true.
func TestLoadAutoRestoredCheckpoint_ErrorWithFailFast(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	expectedErr := errors.New("checkpoint load failed")

	mockCheckpointer := &mockCheckpointer{
		loadFunc: func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
			return nil, expectedErr
		},
	}

	cfg := &ExecutorConfig[any, string]{
		RunID:        "test-run",
		Checkpointer: mockCheckpointer,
	}

	runCfg := &runConfig{
		autoRestore:         true,
		failOnCheckpointErr: true,
	}

	result := &checkpointRestoreResult{}
	yieldCalled := false
	var yieldedErr error

	yield := func(_ string, err error) bool {
		yieldCalled = true
		yieldedErr = err
		return true
	}

	ok := loadAutoRestoredCheckpoint(ctx, cfg, runCfg, result, yield)

	if ok {
		t.Fatal("expected loadAutoRestoredCheckpoint to return false on error with failOnCheckpointErr=true")
	}
	if !yieldCalled {
		t.Fatal("expected yield to be called with error")
	}
	if yieldedErr == nil || !errors.Is(yieldedErr, expectedErr) {
		t.Errorf("expected yielded error to wrap %v, got %v", expectedErr, yieldedErr)
	}
}

// TestLoadAutoRestoredCheckpoint_ErrorContinueWithoutRestore tests error handling with failOnCheckpointErr=false.
func TestLoadAutoRestoredCheckpoint_ErrorContinueWithoutRestore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockCheckpointer := &mockCheckpointer{
		loadFunc: func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
			return nil, errors.New("checkpoint not found")
		},
	}

	cfg := &ExecutorConfig[any, string]{
		RunID:        "test-run",
		Checkpointer: mockCheckpointer,
	}

	runCfg := &runConfig{
		autoRestore:         true,
		failOnCheckpointErr: false, // Continue without restore
	}

	result := &checkpointRestoreResult{}
	yieldCalled := false

	yield := func(_ string, err error) bool {
		yieldCalled = true
		return true
	}

	ok := loadAutoRestoredCheckpoint(ctx, cfg, runCfg, result, yield)

	if !ok {
		t.Fatal("expected loadAutoRestoredCheckpoint to continue (return true) when failOnCheckpointErr=false")
	}
	if yieldCalled {
		t.Fatal("expected yield NOT to be called when continuing without restore")
	}
	if result.State != nil {
		t.Fatal("expected state to remain nil when checkpoint load fails")
	}
}

// TestLoadAutoRestoredCheckpoint_NoAutoRestore tests that auto-restore is skipped when disabled.
func TestLoadAutoRestoredCheckpoint_NoAutoRestore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockCheckpointer := &mockCheckpointer{
		loadFunc: func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
			t.Fatal("checkpointer should not be called when autoRestore=false")
			return nil, nil
		},
	}

	cfg := &ExecutorConfig[any, string]{
		RunID:        "test-run",
		Checkpointer: mockCheckpointer,
	}

	runCfg := &runConfig{
		autoRestore: false, // Disabled
	}

	result := &checkpointRestoreResult{}
	yield := func(_ string, _ error) bool { return true }

	ok := loadAutoRestoredCheckpoint(ctx, cfg, runCfg, result, yield)

	if !ok {
		t.Fatal("expected success when autoRestore is disabled")
	}
	if result.State != nil {
		t.Fatal("expected state to remain nil when autoRestore is disabled")
	}
}

// TestApplyExplicitCheckpoint tests applying an explicitly provided checkpoint.
func TestApplyExplicitCheckpoint(t *testing.T) {
	t.Parallel()

	explicitState := map[string]any{"explicit": true, "value": 42}

	runCfg := &runConfig{
		checkpoint: &checkpoint.Checkpoint{
			State:     explicitState,
			Committed: true,
		},
	}

	result := &checkpointRestoreResult{}

	applyExplicitCheckpoint(runCfg, result)

	if result.State == nil {
		t.Fatal("expected state to be populated from explicit checkpoint")
	}
	if !reflect.DeepEqual(result.State, explicitState) {
		t.Errorf("expected state %v, got %v", explicitState, result.State)
	}
}

// TestApplyExplicitCheckpoint_NoCheckpoint tests that nothing happens when no checkpoint is provided.
func TestApplyExplicitCheckpoint_NoCheckpoint(t *testing.T) {
	t.Parallel()

	runCfg := &runConfig{
		checkpoint: nil, // No checkpoint
	}

	result := &checkpointRestoreResult{
		State: map[string]any{"existing": true},
	}

	applyExplicitCheckpoint(runCfg, result)

	// State should remain unchanged
	if val, ok := result.State["existing"]; !ok || val != true {
		t.Error("expected existing state to remain unchanged")
	}
}

// TestApplyStateUpdates tests applying state updates for human-in-the-loop.
func TestApplyStateUpdates(t *testing.T) {
	t.Parallel()

	runCfg := &runConfig{
		stateUpdates: map[string]any{
			"human_input": "approved",
			"counter":     5,
		},
	}

	result := &checkpointRestoreResult{
		State: map[string]any{"existing": "value"},
	}

	applyStateUpdates(runCfg, result)

	if result.State["human_input"] != "approved" {
		t.Error("expected human_input to be applied")
	}
	if result.State["counter"] != 5 {
		t.Error("expected counter to be applied")
	}
	if result.State["existing"] != "value" {
		t.Error("expected existing state to be preserved")
	}
}

// TestApplyStateUpdates_OverwritesExisting tests that updates overwrite existing keys.
func TestApplyStateUpdates_OverwritesExisting(t *testing.T) {
	t.Parallel()

	runCfg := &runConfig{
		stateUpdates: map[string]any{
			"key": "new_value",
		},
	}

	result := &checkpointRestoreResult{
		State: map[string]any{"key": "old_value"},
	}

	applyStateUpdates(runCfg, result)

	if result.State["key"] != "new_value" {
		t.Errorf("expected key to be overwritten to new_value, got %v", result.State["key"])
	}
}

// TestApplyStateUpdates_CopyOnWrite tests that state is cloned when updates are applied.
func TestApplyStateUpdates_CopyOnWrite(t *testing.T) {
	t.Parallel()

	originalState := map[string]any{"original": true}

	runCfg := &runConfig{
		stateUpdates: map[string]any{"new": true},
	}

	result := &checkpointRestoreResult{
		State:      originalState,
		stateOwned: false, // Shared reference
	}

	applyStateUpdates(runCfg, result)

	// Original state should not be modified
	if _, exists := originalState["new"]; exists {
		t.Error("expected original state to remain unmodified (copy-on-write)")
	}

	// Result state should have the update
	if result.State["new"] != true {
		t.Error("expected result state to have new key")
	}

	// Verify state was cloned
	if reflect.ValueOf(result.State).Pointer() == reflect.ValueOf(originalState).Pointer() {
		t.Error("expected state to be cloned (different pointer)")
	}
}

// TestApplyStateUpdates_EmptyUpdates tests that nothing happens with empty updates.
func TestApplyStateUpdates_EmptyUpdates(t *testing.T) {
	t.Parallel()

	runCfg := &runConfig{
		stateUpdates: nil, // No updates
	}

	originalState := map[string]any{"existing": true}
	result := &checkpointRestoreResult{
		State:      originalState,
		stateOwned: false,
	}

	applyStateUpdates(runCfg, result)

	// State should remain shared (not cloned) since no updates were applied
	if reflect.ValueOf(result.State).Pointer() != reflect.ValueOf(originalState).Pointer() {
		t.Error("expected state to remain shared when no updates are applied")
	}
}

// mockCheckpointer is a simple mock for testing.
type mockCheckpointer struct {
	loadFunc func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
	saveFunc func(ctx context.Context, cp *checkpoint.Checkpoint) error
}

func (m *mockCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, runID)
	}
	return nil, nil
}

func (m *mockCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, cp)
	}
	return nil
}

func (m *mockCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (m *mockCheckpointer) Delete(ctx context.Context, runID string) error {
	return nil
}

func (m *mockCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (m *mockCheckpointer) ListPendingApprovals(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (m *mockCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error) {
	return nil, nil
}
