package embedding

import "context"

// Vector is a dense vector representation used for embeddings.
// Uses float32 for optimal memory efficiency and SIMD performance.
// Embedding models (OpenAI, Cohere, etc.) internally produce float32 values,
// so no precision is lost compared to float64.
type Vector = []float32

// Embedder converts text into vector embeddings for semantic similarity and retrieval.
type Embedder interface {
	// Embed converts a single text into a vector embedding.
	Embed(ctx context.Context, text string) (Vector, error)

	// EmbedBatch converts multiple texts into vector embeddings efficiently.
	// Implementations should optimize this for batch processing when possible.
	EmbedBatch(ctx context.Context, texts []string) ([]Vector, error)

	// Dimensions returns the dimensionality of the embeddings produced by this embedder.
	Dimensions() int
}
