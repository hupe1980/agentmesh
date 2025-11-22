package plugins

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/cache"
	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// SemanticCachePlugin provides semantic caching using embeddings for similarity matching.
// Unlike the exact-match CachePlugin, this finds semantically similar prompts even if
// they're worded differently.
//
// Example:
//   - "What is Python?" and "Tell me about Python" would hit the same cache entry
//   - Configurable similarity threshold (default: 0.90 = 90% similar)
//
// Backends:
//   - Memory: Fast in-process cache with LRU eviction
//   - Redis: Distributed cache for multiple instances
type SemanticCachePlugin struct {
	plugin.NoopPlugin

	cache cache.Cache
}

// NewSemanticCachePlugin creates a semantic caching plugin with the given backend.
//
// Example with memory backend:
//
//	embedder := openai.NewEmbedder(client)
//	memCache := cache.NewMemory(embedder,
//	    cache.WithSimilarityThreshold(0.85),
//	    cache.WithMaxSize(1000))
//	plugin := plugins.NewSemanticCachePlugin(memCache)
//
// Example with Redis backend:
//
//	import redisCache "github.com/hupe1980/agentmesh/pkg/cache/redis"
//	redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	cache := redisCache.NewCache(redisClient, embedder)
//	plugin := plugins.NewSemanticCachePlugin(cache)
func NewSemanticCachePlugin(cache cache.Cache) *SemanticCachePlugin {
	return &SemanticCachePlugin{
		cache: cache,
	}
}

// BeforeModel checks for a semantically similar cached response.
func (p *SemanticCachePlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	// Try to get from cache
	resp, err := p.cache.Get(ctx, req)
	if err != nil {
		// Log error but don't fail the request - return nil to proceed with model call
		return nil, err
	}

	if resp != nil {
		// Cache hit - short-circuit the model call
		return resp, nil
	}

	// Cache miss - proceed with model call
	return nil, nil
}

// AfterModel stores the model response in the semantic cache.
func (p *SemanticCachePlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	// Store in cache (async to not slow down response)
	go func() {
		// Use background context to avoid cancellation
		bgCtx := context.Background()
		_ = p.cache.Set(bgCtx, req, resp) // Ignore errors in background
	}()

	// Return original response
	return nil, nil
}

// GetStats returns cache statistics (if supported by the backend).
func (p *SemanticCachePlugin) GetStats() any {
	// Check if cache implements Stats method
	type statsProvider interface {
		Stats() cache.Stats
	}

	if sp, ok := p.cache.(statsProvider); ok {
		return sp.Stats()
	}

	return nil
}

// Clear removes all entries from the cache.
func (p *SemanticCachePlugin) Clear(ctx context.Context) error {
	return p.cache.Clear(ctx)
}

// Close releases cache resources.
func (p *SemanticCachePlugin) Close() error {
	return p.cache.Close()
}
