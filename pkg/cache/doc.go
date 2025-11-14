// Package cache provides semantic caching capabilities for model requests and responses.
//
// Semantic caching uses embeddings to find similar prompts, allowing cache hits
// even when prompts are not identical but semantically equivalent.
//
// # Basic Usage
//
//	// Create a cache backend
//	embedder := openai.NewEmbedder(client, openai.WithModel("text-embedding-3-small"))
//	cache := cache.NewMemory(embedder, cache.WithSimilarityThreshold(0.85))
//
//	// Wrap a model with caching
//	cachedModel := cache.NewCachedModel(baseModel, cache)
//
//	// Use normally - cache is transparent
//	resp, err := model.Last(cachedModel.Generate(ctx, req))
//
// # Cache Backends
//
//   - Memory: Fast in-process cache with LRU eviction
//   - Redis: Distributed cache with Redis vector search
//
// # Configuration
//
//   - SimilarityThreshold: Minimum cosine similarity for cache hits (default: 0.90)
//   - TTL: Time-to-live for cached entries (default: 1 hour)
//   - MaxSize: Maximum number of entries (memory only, default: 1000)
package cache
