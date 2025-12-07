package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hupe1980/agentmesh/pkg/cache"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/redis/go-redis/v9"
)

// Cache is a Redis-backed semantic cache with vector similarity search.
// Requires Redis Stack or Redis Enterprise with RediSearch module.
type Cache struct {
	client    redis.UniversalClient
	embedder  embedding.Embedder
	options   cache.Options
	prefix    string
	indexName string
}

// Option is a functional option for Redis cache configuration.
type Option func(*Cache)

// WithKeyPrefix sets a prefix for all Redis keys (for namespace isolation).
func WithKeyPrefix(prefix string) Option {
	return func(c *Cache) {
		c.prefix = prefix
	}
}

// WithIndexName sets the RediSearch index name for vector search.
func WithIndexName(name string) Option {
	return func(c *Cache) {
		c.indexName = name
	}
}

// NewCache creates a Redis-backed semantic cache.
// The client can be a regular Redis client or a cluster/sentinel client.
func NewCache(client redis.UniversalClient, embedder embedding.Embedder, opts ...any) *Cache {
	c := &Cache{
		client:    client,
		embedder:  embedder,
		options:   cache.DefaultOptions(),
		prefix:    "agentmesh:cache:",
		indexName: "agentmesh_cache_idx",
	}

	// Apply cache options
	var cacheOpts []cache.Option
	var redisOpts []Option

	for _, opt := range opts {
		switch o := opt.(type) {
		case cache.Option:
			cacheOpts = append(cacheOpts, o)
		case Option:
			redisOpts = append(redisOpts, o)
		}
	}

	c.options = cache.ApplyOptions(cacheOpts...)
	for _, opt := range redisOpts {
		opt(c)
	}

	return c
}

// Get retrieves a cached response for a similar request.
func (c *Cache) Get(ctx context.Context, req *model.Request) (*model.Response, error) {
	// Generate cache key
	key := c.options.KeyFunc(req)

	// Compute embedding for the request
	queryVec, err := c.embedder.Embed(ctx, key)
	if err != nil {
		return nil, err
	}

	// Search for similar vectors using RediSearch
	// Note: This is a simplified implementation. Full implementation would use:
	// FT.SEARCH with KNN vector similarity

	// For now, we'll do a simple scan and compute similarity client-side
	// In production, you'd want to use Redis vector search capabilities

	var bestEntry *cache.Entry
	var bestScore float64

	// Scan all keys with our prefix
	iter := c.client.Scan(ctx, 0, c.prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		entryKey := iter.Val()

		// Get the entry
		data, err := c.client.Get(ctx, entryKey).Bytes()
		if err != nil {
			continue
		}

		var entry cache.Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}

		// Check if expired
		if c.options.TTL > 0 && time.Since(entry.Timestamp) > c.options.TTL {
			// Delete expired entry
			c.client.Del(ctx, entryKey)
			continue
		}

		// Compute similarity
		score := embedding.CosineSimilarity(queryVec, entry.Embedding)
		if score >= c.options.SimilarityThreshold && score > bestScore {
			bestEntry = &entry
			bestScore = score
		}
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	if bestEntry != nil {
		// Add cache metadata
		if bestEntry.Response.Metadata == nil {
			bestEntry.Response.Metadata = make(map[string]any)
		}
		bestEntry.Response.Metadata["cache_hit"] = true
		bestEntry.Response.Metadata["cache_similarity"] = bestScore
		bestEntry.Response.Metadata["cache_backend"] = "redis"

		return bestEntry.Response, nil
	}

	return nil, nil // Cache miss
}

// Set stores a response in the cache.
func (c *Cache) Set(ctx context.Context, req *model.Request, resp *model.Response) error {
	// Generate cache key
	key := c.options.KeyFunc(req)
	redisKey := c.prefix + key

	// Compute embedding for the request
	embedding, err := c.embedder.Embed(ctx, key)
	if err != nil {
		return err
	}

	// Create entry
	entry := &cache.Entry{
		Request:   req,
		Response:  resp,
		Embedding: embedding,
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}

	// Serialize entry
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Store in Redis with TTL
	if c.options.TTL > 0 {
		return c.client.Set(ctx, redisKey, data, c.options.TTL).Err()
	}

	return c.client.Set(ctx, redisKey, data, 0).Err()
}

// Clear removes all cached entries with our prefix.
func (c *Cache) Clear(ctx context.Context) error {
	iter := c.client.Scan(ctx, 0, c.prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Close closes the Redis client connection.
func (c *Cache) Close() error {
	return c.client.Close()
}

// Stats returns cache statistics (requires Redis INFO command access).
func (c *Cache) Stats(ctx context.Context) (cache.Stats, error) {
	// Count keys with our prefix
	var count int64
	iter := c.client.Scan(ctx, 0, c.prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		count++
	}
	if err := iter.Err(); err != nil {
		return cache.Stats{}, err
	}

	// Redis doesn't track hits/misses per key, so we return basic stats
	return cache.Stats{
		Size: int(count),
	}, nil
}
