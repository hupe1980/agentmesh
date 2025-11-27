package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"iter"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/model"
)

// CacheMiddleware caches model responses to reduce redundant API calls.
// Uses request content hash as cache key. Only caches complete (non-streaming) responses.
type CacheMiddleware struct {
	cache sync.Map
}

// NewCacheMiddleware creates a new cache middleware.
func NewCacheMiddleware() *CacheMiddleware {
	return &CacheMiddleware{}
}

// Wrap wraps the model executor with caching.
func (m *CacheMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		// Generate cache key from request
		key, err := m.cacheKey(req)
		if err != nil {
			// Can't cache, pass through
			return next.Generate(ctx, req)
		}

		// Check cache
		if cached, ok := m.cache.Load(key); ok {
			if resp, ok := cached.(*model.Response); ok {
				// Return cached response as iterator
				return func(yield func(*model.Response, error) bool) {
					yield(resp, nil)
				}
			}
		}

		// Execute and cache the last response
		return func(yield func(*model.Response, error) bool) {
			var lastResp *model.Response
			for resp, err := range next.Generate(ctx, req) {
				if err != nil {
					yield(nil, err)
					return
				}
				lastResp = resp
				if !yield(resp, nil) {
					return
				}
			}
			// Cache the final response
			if lastResp != nil && !lastResp.Partial {
				m.cache.Store(key, lastResp)
			}
		}
	})
}

// cacheKey generates a cache key from the request.
func (m *CacheMiddleware) cacheKey(req *model.Request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Clear clears the cache.
func (m *CacheMiddleware) Clear() {
	m.cache = sync.Map{}
}

// Size returns the approximate number of cached entries.
func (m *CacheMiddleware) Size() int {
	count := 0
	m.cache.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
