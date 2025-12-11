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

	checkpointCfg := CheckpointConfig{
		RunID:        "test-run",
		Checkpointer: mockCheckpointer,
	}

	runCfg := &runConfig{
		autoRestore: true,
	}

	result := &checkpointRestoreResult{}
	yield := func(_ string, _ error) bool { return true }

	ok := loadAutoRestoredCheckpoint(ctx, checkpointCfg, runCfg, result, yield)

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

	checkpointCfg := CheckpointConfig{
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

	ok := loadAutoRestoredCheckpoint(ctx, checkpointCfg, runCfg, result, yield)

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

	checkpointCfg := CheckpointConfig{
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

	ok := loadAutoRestoredCheckpoint(ctx, checkpointCfg, runCfg, result, yield)

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

	checkpointCfg := CheckpointConfig{
		RunID:        "test-run",
		Checkpointer: mockCheckpointer,
	}

	runCfg := &runConfig{
		autoRestore: false, // Disabled
	}

	result := &checkpointRestoreResult{}
	yield := func(_ string, _ error) bool { return true }

	ok := loadAutoRestoredCheckpoint(ctx, checkpointCfg, runCfg, result, yield)

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

// Note: applyStateUpdates tests removed - state updates are now applied via BSPState.Write
// with reducers for proper merging. See TestResumeMergesInput for integration test.

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
