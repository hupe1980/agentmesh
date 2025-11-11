package checkpoint

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/message"
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
//	compiled.Invoke(ctx, messages,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("test-run"),
//	)
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
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if checkpoint.RunID == "" {
		return fmt.Errorf("checkpoint runID is empty")
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
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
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
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
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
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
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
	if runID == "" {
		return fmt.Errorf("runID is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.checkpoints[runID]; !exists {
		return fmt.Errorf("no checkpoints found for runID %q", runID)
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
		Timestamp: src.Timestamp,
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

	// Deep copy Messages
	if src.Messages != nil {
		dst.Messages = make([]message.Message, len(src.Messages))
		copy(dst.Messages, src.Messages)
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

	// Deep copy Metadata
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v // Note: shallow copy of values
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
