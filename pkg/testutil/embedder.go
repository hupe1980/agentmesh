package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/embedding"
)

// MockEmbedder is a simple embedder that returns deterministic embeddings for testing.
// It produces consistent, hash-like embeddings based on text length and content.
type MockEmbedder struct {
	dims int
}

// NewMockEmbedder creates a new mock embedder with the specified dimensions.
func NewMockEmbedder(dims int) *MockEmbedder {
	return &MockEmbedder{dims: dims}
}

// Embed converts text to a deterministic vector embedding.
// The embedding is based on text length and first character for reproducibility.
func (m *MockEmbedder) Embed(_ context.Context, text string) (embedding.Vector, error) {
	vec := make(embedding.Vector, m.dims)
	for i := range vec {
		vec[i] = float32(len(text)+i) / 100.0
		if text != "" {
			vec[i] += float32(text[0]) / 1000.0
		}
	}
	return vec, nil
}

// EmbedBatch converts multiple texts to vector embeddings.
func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([]embedding.Vector, error) {
	result := make([]embedding.Vector, len(texts))
	for i, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = vec
	}
	return result, nil
}

// Dimensions returns the dimensionality of embeddings produced by this embedder.
func (m *MockEmbedder) Dimensions() int {
	return m.dims
}

// Ensure MockEmbedder implements embedding.Embedder
var _ embedding.Embedder = (*MockEmbedder)(nil)
