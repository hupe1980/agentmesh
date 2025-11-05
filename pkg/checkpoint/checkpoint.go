package checkpoint

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Checkpoint represents a snapshot of graph execution state at a specific point in time.
// It captures all information needed to resume execution from that point.
type Checkpoint struct {
	// RunID uniquely identifies the execution run
	RunID string

	// Superstep is the BSP superstep number when this checkpoint was created
	Superstep int64

	// Timestamp when the checkpoint was created
	Timestamp time.Time

	// State contains all channel values and custom state
	State map[string]any

	// Messages contains the message history
	Messages []message.Message

	// CompletedNodes tracks which nodes have finished execution
	CompletedNodes []string

	// PausedNodes tracks which nodes are paused (e.g., human-in-the-loop)
	PausedNodes []string

	// Metadata for custom checkpoint annotations
	Metadata map[string]any
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

// Config controls checkpoint behavior
type Config struct {
	// Checkpointer is the storage backend
	Checkpointer Checkpointer

	// SaveInterval controls checkpoint frequency:
	//   0 = save after every superstep (default)
	//   1 = save every superstep
	//   N = save every N supersteps
	SaveInterval int

	// AutoRestore automatically loads the last checkpoint on Invoke/Stream if it exists
	AutoRestore bool
}
