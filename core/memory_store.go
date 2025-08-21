package core

// SearchResult represents a retrieved memory item with a relevance score and arbitrary metadata.
type SearchResult struct {
	ID       string
	Content  string
	Score    float64
	Metadata map[string]any
}

// MemoryStore defines persistence + retrieval (search) for conversational
// memory snippets. Implementations can back search with embeddings, keywords
// or any heuristic. Short method names align with other *Store interfaces.
type MemoryStore interface {
	Get(sessionID string) (map[string]any, error)
	Put(sessionID string, delta map[string]any) error
	Search(sessionID string, query string, limit int) ([]SearchResult, error)
	Store(sessionID string, content string, metadata map[string]any) error
	Delete(sessionID string, memoryID string) error
}
