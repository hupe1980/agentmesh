// Package embedding provides interfaces and implementations for converting text into vector embeddings.
//
// # Overview
//
// The embedding package defines the Embedder interface for text-to-vector conversion,
// enabling semantic search, similarity matching, and RAG (Retrieval-Augmented Generation)
// workflows in agent systems. It also provides similarity functions for comparing vectors.
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
//	embedder := openai.NewEmbedder(func(o *openai.Options) {
//	    o.Model = "text-embedding-3-small"
//	})
//	vector, _ := embedder.Embed(ctx, "semantic search query")
//
// # Similarity Functions
//
// Compare vectors using various distance metrics:
//
//	// Cosine similarity (most common for text embeddings)
//	sim := embedding.CosineSimilarity(vecA, vecB)  // Returns [-1, 1]
//
//	// Euclidean distance
//	dist := embedding.EuclideanDistance(vecA, vecB)  // Returns [0, ∞)
//
//	// Generic similarity with configurable metric
//	sim := embedding.Similarity(vecA, vecB, embedding.Cosine)
//
//	// Normalize vectors for dot product similarity
//	normalized := embedding.Normalize(vec)
//
// # Integration with VectorStore
//
// Embeddings are typically used with vector stores:
//
//	embedder := openai.NewEmbedder()
//	store := memory.New()
//	es := vectorstore.NewEmbeddingStore(store, embedder)
//
//	// Add texts (embeddings generated automatically)
//	es.AddTexts(ctx, []string{"doc1", "doc2"}, nil)
//
//	// Search by text query
//	results, _ := es.SearchText(ctx, "query", vectorstore.SearchOptions{K: 10})
//
// # Batch Processing
//
// For efficiency, use EmbedBatch when processing multiple texts:
//
//	texts := []string{"doc1", "doc2", "doc3"}
//	embeddings, _ := embedder.EmbedBatch(ctx, texts)
//	// Returns []Vector with one vector per input text
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
// Embedder implementations must be safe for concurrent use.
//
// # See Also
//
//   - pkg/vectorstore: Vector storage and similarity search
//   - pkg/retrieval: Document retrieval for RAG workflows
//   - examples/semantic_caching: Complete embedding examples
package embedding
