package retrieval

import "context"

// Document represents a single piece of retrieved content and its metadata.
type Document struct {
	PageContent string         `json:"page_content"`
	Score       float64        `json:"score"`
	Metadata    map[string]any `json:"metadata"`
}

// Retriever defines the contract for retrieving documents for a given query.
type Retriever interface {
	Retrieve(ctx context.Context, query string) ([]Document, error)
}
