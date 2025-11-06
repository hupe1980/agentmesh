package embedding

import "context"

// Embedder converts text into vector embeddings for semantic similarity and retrieval.
type Embedder interface {
	// Embed converts a single text into a vector embedding.
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch converts multiple texts into vector embeddings efficiently.
	// Implementations should optimize this for batch processing when possible.
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

	// Dimensions returns the dimensionality of the embeddings produced by this embedder.
	Dimensions() int
}
