package core

// SearchResult represents a retrieved memory item with a relevance score and arbitrary metadata.
type SearchResult struct {
	ID       string
	Content  string
	Score    float64
	Metadata map[string]any
}
