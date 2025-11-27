package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

// CacheMiddleware caches tool execution results to avoid redundant calls.
// Uses call content hash as cache key.
type CacheMiddleware struct {
	cache sync.Map
}

// NewCacheMiddleware creates a new cache middleware.
func NewCacheMiddleware() *CacheMiddleware {
	return &CacheMiddleware{}
}

// Wrap wraps the tool executor with caching.
func (m *CacheMiddleware) Wrap(next tool.Executor) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		results := make([]tool.ExecutionResult, len(calls))
		uncachedCalls := []tool.Call{}
		uncachedIndices := []int{}

		// Check cache for each call
		for i, call := range calls {
			key, err := m.cacheKey(call)
			if err != nil {
				// Can't cache this call
				uncachedCalls = append(uncachedCalls, call)
				uncachedIndices = append(uncachedIndices, i)
				continue
			}

			if cached, ok := m.cache.Load(key); ok {
				if result, ok := cached.(tool.ExecutionResult); ok {
					results[i] = result
					continue
				}
			}

			// Not in cache
			uncachedCalls = append(uncachedCalls, call)
			uncachedIndices = append(uncachedIndices, i)
		}

		// Execute uncached calls
		if len(uncachedCalls) > 0 { //nolint:nestif // acceptable complexity for cache management
			uncachedResults, err := next.Execute(ctx, uncachedCalls)
			if err != nil {
				return nil, err
			}

			// Store results in cache and in result slice
			for i, result := range uncachedResults {
				idx := uncachedIndices[i]
				results[idx] = result

				// Cache successful results
				if result.Error == nil {
					key, err := m.cacheKey(uncachedCalls[i])
					if err == nil {
						m.cache.Store(key, result)
					}
				}
			}
		}

		return results, nil
	})
}

// cacheKey generates a cache key from a tool call.
func (m *CacheMiddleware) cacheKey(call tool.Call) (string, error) {
	data, err := json.Marshal(call)
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
