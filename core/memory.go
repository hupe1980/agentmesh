package core

import (
	"context"
	"time"
)

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
