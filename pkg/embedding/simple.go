package embedding

import (
	"context"
	"hash/fnv"
	"math"

	"github.com/hupe1980/agentmesh/internal/safeconv"
)

// SimpleEmbedder provides a basic deterministic embedder for testing.
// It uses hash-based pseudo-embeddings that preserve some semantic properties.
// For production use, integrate with real embedding models (OpenAI, Anthropic, etc.).
type SimpleEmbedder struct {
	dimensions int
}

// NewSimpleEmbedder creates a new simple embedder with the specified dimensions.
func NewSimpleEmbedder(dimensions int) *SimpleEmbedder {
	if dimensions <= 0 {
		dimensions = 128 // Default dimension
	}
	return &SimpleEmbedder{
		dimensions: dimensions,
	}
}

// Embed generates a deterministic embedding based on text content.
func (se *SimpleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embedding := make([]float32, se.dimensions)

	if text == "" {
		return embedding, nil
	}

	// Generate multiple hash values for different dimensions
	for i := 0; i < se.dimensions; i++ {
		h := fnv.New64a()
		// Add dimension index to create variation
		_, _ = h.Write([]byte(text))
		_, _ = h.Write([]byte{byte(i)})

		// Convert hash to float in range [-1, 1]
		hash := h.Sum64()
		// Use safe conversion to prevent overflow
		normalized := safeconv.Uint64ToInt64(hash % uint64(math.MaxInt64))
		embedding[i] = float32(normalized) / float32(math.MaxInt64)
	}

	// Normalize the vector
	return normalizeSimple(embedding), nil
}

// EmbedBatch generates embeddings for multiple texts.
func (se *SimpleEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := se.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = embedding
	}
	return result, nil
}

// Dimensions returns the dimensionality of embeddings.
func (se *SimpleEmbedder) Dimensions() int {
	return se.dimensions
}

// normalizeSimple converts a vector to unit length.
func normalizeSimple(vec []float32) []float32 {
	var sum float32
	for _, v := range vec {
		sum += v * v
	}

	if sum == 0 {
		return vec
	}

	norm := float32(math.Sqrt(float64(sum)))
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = v / norm
	}
	return result
}
