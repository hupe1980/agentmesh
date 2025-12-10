package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// MockCheckpointer is a mock implementation of checkpoint.Checkpointer for testing.
type MockCheckpointer struct {
	mu      sync.RWMutex
	storage map[string][]*checkpoint.Checkpoint

	SaveFunc                 func(ctx context.Context, cp *checkpoint.Checkpoint) error
	LoadFunc                 func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
	ListFunc                 func(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error)
	DeleteFunc               func(ctx context.Context, runID string) error
	LoadAtSuperstepFunc      func(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error)
	ListPendingApprovalsFunc func(ctx context.Context) ([]*checkpoint.Checkpoint, error)
	GetApprovalHistoryFunc   func(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error)
}

// NewMockCheckpointer creates a new MockCheckpointer with in-memory storage.
func NewMockCheckpointer() *MockCheckpointer {
	return &MockCheckpointer{
		storage: make(map[string][]*checkpoint.Checkpoint),
	}
}

// Save persists a checkpoint.
func (m *MockCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, cp)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.storage[cp.RunID] = append(m.storage[cp.RunID], cp)
	return nil
}

// Load retrieves the most recent checkpoint for a run ID.
func (m *MockCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	if m.LoadFunc != nil {
		return m.LoadFunc(ctx, runID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.storage[runID]
	if len(checkpoints) == 0 {
		return nil, nil
	}

	return checkpoints[len(checkpoints)-1], nil
}

// List returns all checkpoints for a run ID.
func (m *MockCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, runID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.storage[runID]
	if len(checkpoints) == 0 {
		return []*checkpoint.Checkpoint{}, nil
	}

	// Return copy in reverse order (newest first)
	result := make([]*checkpoint.Checkpoint, len(checkpoints))
	for i, cp := range checkpoints {
		result[len(checkpoints)-1-i] = cp
	}

	return result, nil
}

// Delete removes all checkpoints for a run ID.
func (m *MockCheckpointer) Delete(ctx context.Context, runID string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, runID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.storage[runID]; !exists {
		return errors.New("no checkpoints found")
	}

	delete(m.storage, runID)
	return nil
}

// LoadAtSuperstep retrieves a checkpoint at a specific superstep.
func (m *MockCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	if m.LoadAtSuperstepFunc != nil {
		return m.LoadAtSuperstepFunc(ctx, runID, superstep)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.storage[runID]
	for _, cp := range checkpoints {
		if cp.Superstep == superstep {
			return cp, nil
		}
	}

	return nil, nil
}

// ListPendingApprovals returns all checkpoints with pending approvals.
func (m *MockCheckpointer) ListPendingApprovals(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	if m.ListPendingApprovalsFunc != nil {
		return m.ListPendingApprovalsFunc(ctx)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*checkpoint.Checkpoint
	for _, checkpoints := range m.storage {
		if len(checkpoints) > 0 {
			latest := checkpoints[len(checkpoints)-1]
			if latest.ApprovalMetadata != nil && len(latest.ApprovalMetadata.PendingApprovals) > 0 {
				result = append(result, latest)
			}
		}
	}

	return result, nil
}

// GetApprovalHistory retrieves the approval history for a run ID.
func (m *MockCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error) {
	if m.GetApprovalHistoryFunc != nil {
		return m.GetApprovalHistoryFunc(ctx, runID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.storage[runID]
	if len(checkpoints) == 0 {
		return []checkpoint.ApprovalRecord{}, nil
	}

	for i := len(checkpoints) - 1; i >= 0; i-- {
		cp := checkpoints[i]
		if cp.ApprovalMetadata != nil && len(cp.ApprovalMetadata.ApprovalHistory) > 0 {
			return cp.ApprovalMetadata.ApprovalHistory, nil
		}
	}

	return []checkpoint.ApprovalRecord{}, nil
}

// CheckpointCount returns the number of checkpoints for a run ID.
func (m *MockCheckpointer) CheckpointCount(runID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.storage[runID])
}

// AllCheckpoints returns all stored checkpoints for a run ID.
func (m *MockCheckpointer) AllCheckpoints(runID string) []*checkpoint.Checkpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*checkpoint.Checkpoint, len(m.storage[runID]))
	copy(result, m.storage[runID])
	return result
}

// Reset clears all stored checkpoints.
func (m *MockCheckpointer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storage = make(map[string][]*checkpoint.Checkpoint)
}

// Ensure MockCheckpointer implements checkpoint.Checkpointer
var _ checkpoint.Checkpointer = (*MockCheckpointer)(nil)
