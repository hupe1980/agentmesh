package embedding

import (
	"context"
	"hash/fnv"
	"math"
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
func (se *SimpleEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	embedding := make([]float64, se.dimensions)

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
		// Use modulo to prevent overflow instead of direct conversion
		normalized := int64(hash % uint64(math.MaxInt64))
		embedding[i] = float64(normalized) / float64(math.MaxInt64)
	}

	// Normalize the vector
	return normalize(embedding), nil
}

// EmbedBatch generates embeddings for multiple texts.
func (se *SimpleEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
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

// normalize converts a vector to unit length.
func normalize(vec []float64) []float64 {
	var sum float64
	for _, v := range vec {
		sum += v * v
	}

	if sum == 0 {
		return vec
	}

	norm := math.Sqrt(sum)
	result := make([]float64, len(vec))
	for i, v := range vec {
		result[i] = v / norm
	}
	return result
}
