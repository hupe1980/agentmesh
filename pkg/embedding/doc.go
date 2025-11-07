// Package embedding provides interfaces and implementations for converting text into vector embeddings.
//
// # Overview
//
// The embedding package defines the Embedder interface for text-to-vector conversion,
// enabling semantic search, similarity matching, and RAG (Retrieval-Augmented Generation)
// workflows in agent systems.
//
// # Available Implementations
//
//   - SimpleEmbedder: Deterministic hash-based embeddings (testing/development)
//   - OpenAIEmbedder: Production embeddings via OpenAI API (pkg/embedding/openai)
//
// # Basic Usage
//
//	// Development/testing with deterministic embeddings
//	embedder := embedding.NewSimpleEmbedder(768)  // 768 dimensions
//	vector, _ := embedder.Embed(ctx, "Hello, world!")
//	fmt.Printf("Embedding dimensions: %d\n", len(vector))
//
//	// Production with OpenAI embeddings
//	embedder := openai.NewEmbedder(client, openai.WithModel("text-embedding-3-small"))
//	vector, _ := embedder.Embed(ctx, "semantic search query")
//
// # Batch Processing
//
// For efficiency, use EmbedBatch when processing multiple texts:
//
//	texts := []string{"doc1", "doc2", "doc3"}
//	embeddings, _ := embedder.EmbedBatch(ctx, texts)
//	// Returns [][]float64 with one vector per input text
//
// # Integration with RAG
//
// Embeddings are typically used in conjunction with retrieval systems:
//
//	// 1. Embed documents for indexing
//	docs := []string{"The sky is blue", "Grass is green"}
//	docVectors, _ := embedder.EmbedBatch(ctx, docs)
//
//	// 2. Embed query for similarity search
//	queryVector, _ := embedder.Embed(ctx, "What color is the sky?")
//
//	// 3. Compute similarity (cosine, dot product, etc.)
//	similarity := cosineSimilarity(queryVector, docVectors[0])
//
// # Dimension Consistency
//
// All vectors from the same embedder have consistent dimensions:
//
//	dims := embedder.Dimensions()  // e.g., 768, 1536, 3072
//	// All embeddings will be length dims
//
// # Thread Safety
//
// Embedder implementations must be safe for concurrent use. The interface
// does not prescribe internal locking strategies, but implementations should
// handle concurrent Embed/EmbedBatch calls safely.
//
// # See Also
//
//   - pkg/retrieval: Document retrieval for RAG workflows
//   - pkg/agent: RAG agent patterns using embeddings
//   - examples/openai_embedder: Complete embedding examples
package embedding
