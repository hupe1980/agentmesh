package graph

import (
	"context"
	"fmt"
	"time"
)

// NodeConfig holds execution policies for a node.
// Policies are applied by the executor when running the node.
type NodeConfig struct {
	RetryPolicy *RetryPolicy
	CachePolicy *CachePolicy
}

// CachePolicy configures caching behavior for node execution.
// Cached results are stored and retrieved based on a cache key.
type CachePolicy struct {
	// Enabled determines if caching is active.
	Enabled bool

	// TTL is the time-to-live for cached results.
	// Zero means cache entries never expire.
	TTL time.Duration

	// KeyFunc generates a cache key from the current state.
	// The key uniquely identifies the input to this node execution.
	// If nil, caching is effectively disabled.
	KeyFunc func(ctx context.Context, snapshot map[string]any) string

	// Store is the cache backend. If nil, uses in-memory cache.
	Store CacheStore
}

// CacheStore is the interface for cache backends.
type CacheStore interface {
	// Get retrieves a cached value by key.
	// Returns nil if not found or expired.
	Get(ctx context.Context, key string) (CacheEntry, error)

	// Set stores a value in the cache with the given TTL.
	Set(ctx context.Context, key string, entry CacheEntry, ttl time.Duration) error

	// Delete removes a cached value.
	Delete(ctx context.Context, key string) error
}

// CacheEntry represents a cached node execution result.
type CacheEntry struct {
	// Updates are the state updates returned by the node.
	Updates map[string]any

	// Timestamp is when this entry was cached.
	Timestamp time.Time

	// Metadata stores additional cache information.
	Metadata map[string]any
}

// NodeOption configures a node's execution policies.
type NodeOption func(*NodeConfig)

// WithRetryPolicy sets the retry policy for a node.
//
// Example:
//
//	g.AddNode("api_call", apiNode,
//	    WithRetryPolicy(&RetryPolicy{
//	        MaxAttempts: 3,
//	        Backoff: ExponentialBackoff(100 * time.Millisecond),
//	    }))
func WithRetryPolicy(policy *RetryPolicy) NodeOption {
	return func(c *NodeConfig) {
		c.RetryPolicy = policy
	}
}

// WithCachePolicy sets the cache policy for a node.
//
// Example:
//
//	g.AddNode("expensive_op", expensiveNode,
//	    WithCachePolicy(&CachePolicy{
//	        Enabled: true,
//	        TTL: 5 * time.Minute,
//	        KeyFunc: func(ctx context.Context, state map[string]any) string {
//	            return fmt.Sprintf("op:%v", state["input"])
//	        },
//	    }))
func WithCachePolicy(policy *CachePolicy) NodeOption {
	return func(c *NodeConfig) {
		c.CachePolicy = policy
	}
}

// defaultNodeConfig returns a NodeConfig with sensible defaults.
func defaultNodeConfig() *NodeConfig {
	return &NodeConfig{
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 1, // No retries by default
			Backoff:     nil,
			Retryable:   nil, // All errors retryable
		},
		CachePolicy: &CachePolicy{
			Enabled: false,
		},
	}
}

// Validate checks if the node configuration is valid.
func (c *NodeConfig) Validate() error {
	if c.RetryPolicy != nil {
		if c.RetryPolicy.MaxAttempts < 1 {
			return fmt.Errorf("retry policy: MaxAttempts must be >= 1, got %d", c.RetryPolicy.MaxAttempts)
		}
	}

	if c.CachePolicy != nil {
		if c.CachePolicy.Enabled && c.CachePolicy.KeyFunc == nil {
			return fmt.Errorf("cache policy: KeyFunc is required when caching is enabled")
		}
	}

	return nil
}
