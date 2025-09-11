package core

import (
	"context"
	"time"
)

// ArtifactStore persists arbitrary binary artifacts scoped by session.
// Implementations should be thread-safe. Method names mirror other stores.
type ArtifactStore interface {
	// Save persists a binary artifact to the store.
	Save(ctx context.Context, appName, userID, sessionID, fileName string, artifact Part) error

	// Load retrieves a binary artifact from the store.
	Load(ctx context.Context, appName, userID, sessionID, fileName string) (Part, error)

	// ListKeys retrieves all keys for a given session.
	ListKeys(ctx context.Context, appName, userID, sessionID string) ([]string, error)

	// Delete removes a binary artifact from the store.
	Delete(ctx context.Context, appName, userID, sessionID, fileName string) error

	// Close releases any resources held by the store.
	Close() error
}

// MemoryItem represents a single memory entry captured from a conversation
// with the associated author and timestamp.
type MemoryItem struct {
	// The parts stored as memory
	Parts []Part

	// Who produced the parts (e.g., user, assistant)
	Author string

	// When the parts were created
	Timestamp time.Time
}

// SearchResult represents the outcome of a memory search operation.
// It includes the matched parts and related memory entries.
type SearchResult struct {
	// Memories is a list of memory entries that relate to the search query.
	Memories []*MemoryItem
}

// MemoryStore persists and retrieves conversational memory snippets.
// Search semantics are implementation-defined (embeddings, keywords, etc.).
type MemoryStore interface {
	// AddSession adds a new session to the store.
	AddSession(ctx context.Context, session *Session) error

	// Search retrieves memory snippets that match the query.
	Search(ctx context.Context, appName, userID string, query string) (*SearchResult, error)

	// Close releases any resources held by the store.
	Close() error
}

// SessionStore persists sessions and their evolving event history.
type SessionStore interface {
	// GetOrCreate retrieves an existing session or creates a new one.
	GetOrCreate(ctx context.Context, appName, userID, sessionID string) (*Session, error)

	// AppendEvent adds a new event to the session's event history.
	AppendEvent(ctx context.Context, session *Session, event *Event) error

	// Close releases any resources held by the store.
	Close() error
}
