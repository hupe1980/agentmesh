package embedding

import "context"

// Vector is a dense vector representation used for embeddings.
type Vector = []float64

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
