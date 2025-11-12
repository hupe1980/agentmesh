package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// CachePlugin caches model responses to avoid redundant API calls.
// It uses a simple in-memory cache with configurable size.
type CachePlugin struct {
	callbacks.NoopPlugin

	mu        sync.RWMutex
	cache     map[string]*model.Response
	maxSize   int
	hits      int64
	misses    int64
	evictions int64
}

// NewCachePlugin creates a caching plugin.
// maxSize is the maximum number of responses to cache (0 = unlimited).
func NewCachePlugin(maxSize int) *CachePlugin {
	return &CachePlugin{
		cache:   make(map[string]*model.Response),
		maxSize: maxSize,
	}
}

// BeforeModel returns cached response if available.
func (p *CachePlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	key := p.cacheKey(req)

	p.mu.RLock()
	cached, ok := p.cache[key]
	p.mu.RUnlock()

	if ok {
		p.mu.Lock()
		p.hits++
		p.mu.Unlock()

		// Return cached response (short-circuit)
		return cached, nil
	}

	p.mu.Lock()
	p.misses++
	p.mu.Unlock()

	return nil, nil // Cache miss, proceed with model call
}

// AfterModel caches the model response for future requests.
func (p *CachePlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	key := p.cacheKey(req)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if we need to evict entries
	if p.maxSize > 0 && len(p.cache) >= p.maxSize {
		// Simple eviction: remove a random entry (first in map iteration)
		for k := range p.cache {
			delete(p.cache, k)
			p.evictions++
			break
		}
	}

	// Store response in cache
	p.cache[key] = resp

	return nil, nil // Keep original response
}

// cacheKey generates a cache key from the request.
// It hashes the messages to create a unique identifier.
func (p *CachePlugin) cacheKey(req *model.Request) string {
	h := sha256.New()

	// Hash messages
	for _, msg := range req.Messages {
		h.Write([]byte(msg.Type()))
		h.Write([]byte(message.Stringify(msg)))
	}

	// Hash system prompt
	if req.SystemPrompt != "" {
		h.Write([]byte(req.SystemPrompt))
	}

	// Hash tools (if any)
	if len(req.Tools) > 0 {
		toolsJSON, _ := json.Marshal(req.Tools)
		h.Write(toolsJSON)
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// GetStats returns cache statistics.
func (p *CachePlugin) GetStats() CacheStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := p.hits + p.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(p.hits) / float64(total)
	}

	return CacheStats{
		Size:      len(p.cache),
		Hits:      p.hits,
		Misses:    p.misses,
		HitRate:   hitRate,
		Evictions: p.evictions,
	}
}

// CacheStats contains cache performance metrics.
type CacheStats struct {
	Size      int
	Hits      int64
	Misses    int64
	HitRate   float64
	Evictions int64
}

// Clear removes all entries from the cache.
func (p *CachePlugin) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cache = make(map[string]*model.Response)
}
