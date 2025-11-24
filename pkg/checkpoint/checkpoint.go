package checkpoint

import (
	"context"
	"time"
)

// PendingWrite represents a state update that has been produced by a node
// but not yet applied to the graph state. This enables two-phase commit semantics
// for checkpointing: save pending writes before applying them, allowing for
// fine-grained interrupts and human review before state changes take effect.
//
// Use cases:
//   - Interrupt after node execution, before state application
//   - Human review of pending changes before committing
//   - Transactional semantics (all-or-nothing updates)
//   - Audit trail of what was written vs what was applied
type PendingWrite struct {
	// NodeName is the node that produced this write
	NodeName string `json:"nodeName"`

	// Channel is the state channel being updated
	Channel string `json:"channel"`

	// Value is the update value to be applied
	Value any `json:"value"`

	// Timestamp when this write was created
	Timestamp time.Time `json:"timestamp"`
}

// Checkpoint represents a snapshot of graph execution state at a specific point in time.
// It captures all information needed to resume execution from that point.
type Checkpoint struct {
	// RunID uniquely identifies the execution run
	RunID string `json:"runID"`

	// Superstep is the BSP superstep number when this checkpoint was created
	Superstep int64 `json:"superstep"`

	// Version is a monotonically increasing counter for checkpoint validation.
	// Each state mutation increments the version, enabling detection of checkpoint corruption,
	// concurrent modifications, or incorrect restore sequences.
	Version uint64 `json:"version"`

	// Timestamp when the checkpoint was created
	Timestamp time.Time `json:"timestamp"`

	// Signature is an HMAC-SHA256 signature of the checkpoint data for integrity verification.
	// When signing is enabled, this field is populated during Save() and verified during Load().
	// An empty signature indicates the checkpoint was saved without signing enabled.
	Signature []byte `json:"signature,omitempty"`

	// State contains all channel values including message history (via MessagesKey),
	// conversation state, and any custom state registered with the state manager.
	// Message history is stored in state, not as a separate Messages field.
	State map[string]any `json:"state"`

	// CompletedNodes tracks which nodes have finished execution.
	// Needed for smart resume: skip re-executing completed nodes.
	CompletedNodes []string `json:"completedNodes"`

	// PausedNodes tracks which nodes are paused (e.g., waiting for human input).
	// Critical for human-in-the-loop workflows: resume from the exact pause point.
	PausedNodes []string `json:"pausedNodes,omitempty"`

	// PendingWrites are state updates produced by nodes but not yet applied.
	// Used for two-phase commit: checkpoint after node execution but before
	// state application. Enables fine-grained interrupts and human review.
	// When resuming, these writes are applied first before continuing execution.
	PendingWrites []PendingWrite `json:"pendingWrites,omitempty"`

	// Committed indicates whether PendingWrites have been applied to state.
	// Two-phase commit protocol:
	//   1. Save checkpoint with PendingWrites (Committed=false)
	//   2. Apply PendingWrites to state
	//   3. Update checkpoint (Committed=true)
	// On resume: only apply PendingWrites if Committed=false to prevent double-application.
	Committed bool `json:"committed"`

	// Metadata for custom checkpoint annotations
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Checkpointer defines the interface for checkpoint persistence.
// Implementations can use any storage backend (in-memory, SQLite, PostgreSQL, Redis, etc.)
type Checkpointer interface {
	// Save persists a checkpoint for the given run ID.
	// Returns error if save fails.
	Save(ctx context.Context, checkpoint *Checkpoint) error

	// Load retrieves the most recent checkpoint for the given run ID.
	// Returns nil checkpoint if no checkpoint exists (first run).
	// Returns error if load fails.
	Load(ctx context.Context, runID string) (*Checkpoint, error)

	// List returns all checkpoints for a run ID, ordered by superstep (newest first).
	// Returns empty slice if no checkpoints exist.
	// Returns error if listing fails.
	List(ctx context.Context, runID string) ([]*Checkpoint, error)

	// Delete removes all checkpoints for a run ID.
	// Returns error if deletion fails or no checkpoints found.
	Delete(ctx context.Context, runID string) error

	// LoadAtSuperstep retrieves a checkpoint at a specific superstep.
	// Useful for time-travel debugging.
	// Returns nil if no checkpoint exists at that superstep.
	// Returns error if load fails.
	LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*Checkpoint, error)
}

// Option is a functional option for configuring checkpoint behavior
type Option func(*options)

// options holds checkpoint configuration
type options struct {
	checkpointer Checkpointer
	saveInterval int
	autoRestore  bool
}

// WithCheckpointer sets the storage backend for checkpoints
func WithCheckpointer(checkpointer Checkpointer) Option {
	return func(o *options) {
		o.checkpointer = checkpointer
	}
}

// WithSaveInterval controls checkpoint frequency:
//
//	0 = save after every superstep (default)
//	1 = save every superstep
//	N = save every N supersteps
func WithSaveInterval(interval int) Option {
	return func(o *options) {
		o.saveInterval = interval
	}
}

// WithAutoRestore automatically loads the last checkpoint on Invoke/Stream if it exists
func WithAutoRestore(enabled bool) Option {
	return func(o *options) {
		o.autoRestore = enabled
	}
}

// ApplyOptions applies checkpoint options to RunOptions (used by graph package)
func ApplyOptions(opts []Option) (Checkpointer, int, bool) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o.checkpointer, o.saveInterval, o.autoRestore
}
