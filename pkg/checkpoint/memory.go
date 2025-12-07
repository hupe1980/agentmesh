package checkpoint

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryCheckpointer implements Checkpointer using an in-memory store.
// This is useful for testing and development. Data is lost when the process exits.
//
// Thread-safe for concurrent access.
type InMemoryCheckpointer struct {
	mu          sync.RWMutex
	checkpoints map[string][]*Checkpoint // runID -> checkpoints (sorted by superstep)
	signingKey  []byte                   // Optional HMAC signing key for checkpoint integrity
}

// InMemoryCheckpointerOption is a functional option for configuring InMemoryCheckpointer.
type InMemoryCheckpointerOption func(*InMemoryCheckpointer)

// WithSigning configures the checkpointer to sign checkpoints on save and verify signatures on load.
// The signing key should be a secure random value (at least 32 bytes recommended).
//
// Example:
//
//	signingKey := []byte("your-secure-signing-key-at-least-32-bytes-long")
//	checkpointer := checkpoint.NewInMemoryCheckpointer(checkpoint.WithSigning(signingKey))
func WithSigning(key []byte) InMemoryCheckpointerOption {
	return func(c *InMemoryCheckpointer) {
		c.signingKey = key
	}
}

// NewInMemoryCheckpointer creates a new in-memory checkpointer.
//
// Example:
//
//	checkpointer := checkpoint.NewInMemoryCheckpointer()
//	result, _ := graph.Last(compiled.Run(ctx, messages,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("test-run"),
//	))
//
// With signing enabled:
//
//	signingKey := []byte("your-secure-signing-key")
//	checkpointer := checkpoint.NewInMemoryCheckpointer(checkpoint.WithSigning(signingKey))
func NewInMemoryCheckpointer(opts ...InMemoryCheckpointerOption) *InMemoryCheckpointer {
	c := &InMemoryCheckpointer{
		checkpoints: make(map[string][]*Checkpoint),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Save persists a checkpoint to memory
func (m *InMemoryCheckpointer) Save(ctx context.Context, checkpoint *Checkpoint) error {
	// Early context check to fail fast if already cancelled
	if err := ctx.Err(); err != nil {
		return err
	}

	if checkpoint == nil {
		return ErrNilCheckpoint
	}
	if checkpoint.RunID == "" {
		return ErrEmptyRunID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Deep copy the checkpoint to prevent external modifications
	cp := m.deepCopy(checkpoint)

	// Sign the checkpoint if signing key is configured
	if len(m.signingKey) > 0 {
		signature, err := SignCheckpoint(cp, m.signingKey)
		if err != nil {
			return fmt.Errorf("failed to sign checkpoint: %w", err)
		}
		cp.Signature = signature
	}

	runCheckpoints, exists := m.checkpoints[cp.RunID]
	if !exists {
		// First checkpoint for this run
		m.checkpoints[cp.RunID] = []*Checkpoint{cp}
		return nil
	}

	// Check if checkpoint at this superstep already exists (replace it)
	for i, existing := range runCheckpoints {
		if existing.Superstep == cp.Superstep {
			runCheckpoints[i] = cp
			return nil
		}
	}

	// Insert in sorted order (newest first)
	inserted := false
	for i, existing := range runCheckpoints {
		if cp.Superstep > existing.Superstep {
			// Insert before this position
			runCheckpoints = append(runCheckpoints[:i+1], runCheckpoints[i:]...)
			runCheckpoints[i] = cp
			inserted = true
			break
		}
	}

	if !inserted {
		// Append at the end (oldest)
		runCheckpoints = append(runCheckpoints, cp)
	}

	m.checkpoints[cp.RunID] = runCheckpoints
	return nil
}

// Load retrieves the most recent checkpoint for a run ID
func (m *InMemoryCheckpointer) Load(ctx context.Context, runID string) (*Checkpoint, error) {
	// Early context check to fail fast if already cancelled
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if runID == "" {
		return nil, ErrEmptyRunID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	runCheckpoints, exists := m.checkpoints[runID]
	if !exists || len(runCheckpoints) == 0 {
		return nil, nil // No checkpoint found
	}

	// First checkpoint is the most recent (sorted by superstep desc)
	cp := m.deepCopy(runCheckpoints[0])

	// Verify signature if signing key is configured
	if len(m.signingKey) > 0 {
		if err := VerifyCheckpoint(cp, m.signingKey); err != nil {
			return nil, fmt.Errorf("checkpoint signature verification failed: %w", err)
		}
	}

	return cp, nil
}

// LoadAtSuperstep retrieves a checkpoint at a specific superstep
func (m *InMemoryCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*Checkpoint, error) {
	// Early context check to fail fast if already cancelled
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if runID == "" {
		return nil, ErrEmptyRunID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	runCheckpoints, exists := m.checkpoints[runID]
	if !exists {
		return nil, nil // No checkpoints for this run
	}

	for _, cp := range runCheckpoints {
		if cp.Superstep == superstep {
			checkpoint := m.deepCopy(cp)

			// Verify signature if signing key is configured
			if len(m.signingKey) > 0 {
				if err := VerifyCheckpoint(checkpoint, m.signingKey); err != nil {
					return nil, fmt.Errorf("checkpoint signature verification failed: %w", err)
				}
			}

			return checkpoint, nil
		}
	}

	return nil, nil // No checkpoint at this superstep
}

// List returns all checkpoints for a run ID, ordered by superstep descending
func (m *InMemoryCheckpointer) List(ctx context.Context, runID string) ([]*Checkpoint, error) {
	// Early context check to fail fast if already cancelled
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if runID == "" {
		return nil, ErrEmptyRunID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	runCheckpoints, exists := m.checkpoints[runID]
	if !exists || len(runCheckpoints) == 0 {
		return []*Checkpoint{}, nil
	}

	// Return deep copies and verify signatures if signing is enabled
	result := make([]*Checkpoint, len(runCheckpoints))
	for i, cp := range runCheckpoints {
		checkpoint := m.deepCopy(cp)

		// Verify signature if signing key is configured
		if len(m.signingKey) > 0 {
			if err := VerifyCheckpoint(checkpoint, m.signingKey); err != nil {
				return nil, fmt.Errorf("checkpoint signature verification failed for superstep %d: %w", checkpoint.Superstep, err)
			}
		}

		result[i] = checkpoint
	}

	return result, nil
}

// Delete removes all checkpoints for a run ID
func (m *InMemoryCheckpointer) Delete(ctx context.Context, runID string) error {
	// Early context check to fail fast if already cancelled
	if err := ctx.Err(); err != nil {
		return err
	}

	if runID == "" {
		return ErrEmptyRunID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.checkpoints[runID]; !exists {
		return &RunNotFoundError{RunID: runID}
	}

	delete(m.checkpoints, runID)
	return nil
}

// deepCopy creates a deep copy of a checkpoint to prevent external modifications
func (m *InMemoryCheckpointer) deepCopy(src *Checkpoint) *Checkpoint {
	if src == nil {
		return nil
	}

	dst := &Checkpoint{
		RunID:     src.RunID,
		Superstep: src.Superstep,
		Version:   src.Version,
		Timestamp: src.Timestamp,
		Committed: src.Committed,
	}

	// Deep copy Signature
	if src.Signature != nil {
		dst.Signature = make([]byte, len(src.Signature))
		copy(dst.Signature, src.Signature)
	}

	// Deep copy State
	if src.State != nil {
		dst.State = make(map[string]any, len(src.State))
		for k, v := range src.State {
			dst.State[k] = v // Note: shallow copy of values
		}
	}

	// Deep copy CompletedNodes
	if src.CompletedNodes != nil {
		dst.CompletedNodes = make([]string, len(src.CompletedNodes))
		copy(dst.CompletedNodes, src.CompletedNodes)
	}

	// Deep copy PausedNodes
	if src.PausedNodes != nil {
		dst.PausedNodes = make([]string, len(src.PausedNodes))
		copy(dst.PausedNodes, src.PausedNodes)
	}

	// Deep copy PendingWrites
	if src.PendingWrites != nil {
		dst.PendingWrites = make([]PendingWrite, len(src.PendingWrites))
		copy(dst.PendingWrites, src.PendingWrites)
	}

	// Deep copy Metadata
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v // Note: shallow copy of values
		}
	}

	if src.ManagedValues != nil {
		dst.ManagedValues = make([]ManagedValueDescriptor, len(src.ManagedValues))
		copy(dst.ManagedValues, src.ManagedValues)
	}

	// Deep copy ApprovalMetadata
	if src.ApprovalMetadata != nil {
		dst.ApprovalMetadata = &ApprovalMetadata{}

		// Copy PendingApprovals
		if src.ApprovalMetadata.PendingApprovals != nil {
			dst.ApprovalMetadata.PendingApprovals = make(map[string]*PendingApproval, len(src.ApprovalMetadata.PendingApprovals))
			for k, v := range src.ApprovalMetadata.PendingApprovals {
				// Create a copy of the PendingApproval struct
				pendingCopy := *v
				dst.ApprovalMetadata.PendingApprovals[k] = &pendingCopy
			}
		}

		// Copy ApprovalHistory
		if src.ApprovalMetadata.ApprovalHistory != nil {
			dst.ApprovalMetadata.ApprovalHistory = make([]ApprovalRecord, len(src.ApprovalMetadata.ApprovalHistory))
			copy(dst.ApprovalMetadata.ApprovalHistory, src.ApprovalMetadata.ApprovalHistory)
		}
	}

	return dst
}

// Clear removes all checkpoints from memory (useful for testing)
func (m *InMemoryCheckpointer) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpoints = make(map[string][]*Checkpoint)
}

// Stats returns statistics about stored checkpoints
func (m *InMemoryCheckpointer) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)
	for runID, checkpoints := range m.checkpoints {
		stats[runID] = len(checkpoints)
	}
	return stats
}

// ListPendingApprovals returns all checkpoints with pending approvals.
func (m *InMemoryCheckpointer) ListPendingApprovals(ctx context.Context) ([]*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []*Checkpoint
	for _, checkpoints := range m.checkpoints {
		for _, cp := range checkpoints {
			if cp.ApprovalMetadata != nil && len(cp.ApprovalMetadata.PendingApprovals) > 0 {
				pending = append(pending, m.deepCopy(cp))
			}
		}
	}

	return pending, nil
}

// GetApprovalHistory returns the approval history for a specific run.
func (m *InMemoryCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]ApprovalRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints, exists := m.checkpoints[runID]
	if !exists {
		return []ApprovalRecord{}, nil
	}

	// Collect all approval history from all checkpoints in this run
	var history []ApprovalRecord
	for _, cp := range checkpoints {
		if cp.ApprovalMetadata != nil && len(cp.ApprovalMetadata.ApprovalHistory) > 0 {
			history = append(history, cp.ApprovalMetadata.ApprovalHistory...)
		}
	}

	return history, nil
}
