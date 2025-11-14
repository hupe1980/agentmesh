package cache

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// Entry represents a cached model response with its associated prompt and metadata.
type Entry struct {
	// Request is the original model request that was cached
	Request *model.Request

	// Response is the cached model response
	Response *model.Response

	// Embedding is the vector representation of the prompt for similarity search
	Embedding []float64

	// Timestamp is when this entry was cached
	Timestamp time.Time

	// Metadata contains additional cache-specific information
	Metadata map[string]any
}

// Cache defines the interface for semantic caching backends.
// Implementations use embeddings to find similar prompts for cache hits.
type Cache interface {
	// Get retrieves a cached response for a similar request.
	// Returns nil if no sufficiently similar entry exists.
	// The similarity is determined by cosine similarity of request embeddings.
	Get(ctx context.Context, req *model.Request) (*model.Response, error)

	// Set stores a response in the cache for the given request.
	// The request will be embedded and stored with the response.
	Set(ctx context.Context, req *model.Request, resp *model.Response) error

	// Clear removes all entries from the cache.
	Clear(ctx context.Context) error

	// Close releases any resources held by the cache.
	Close() error
}

// Options configures cache behavior.
type Options struct {
	// SimilarityThreshold is the minimum cosine similarity (0.0-1.0) required for a cache hit.
	// Higher values require closer matches. Default: 0.90
	SimilarityThreshold float64

	// TTL is how long cached entries remain valid.
	// Zero means no expiration. Default: 1 hour
	TTL time.Duration

	// MaxSize is the maximum number of entries to cache (memory backend only).
	// Zero means unlimited (not recommended). Default: 1000
	MaxSize int

	// KeyFunc generates a cache key from a request.
	// If nil, uses default key generation (messages + system prompt).
	KeyFunc func(*model.Request) string
}

// Option is a functional option for configuring cache behavior.
type Option func(*Options)

// WithSimilarityThreshold sets the minimum cosine similarity for cache hits.
func WithSimilarityThreshold(threshold float64) Option {
	return func(o *Options) {
		o.SimilarityThreshold = threshold
	}
}

// WithTTL sets the time-to-live for cached entries.
func WithTTL(ttl time.Duration) Option {
	return func(o *Options) {
		o.TTL = ttl
	}
}

// WithMaxSize sets the maximum number of entries (memory backend only).
func WithMaxSize(size int) Option {
	return func(o *Options) {
		o.MaxSize = size
	}
}

// WithKeyFunc sets a custom key generation function.
func WithKeyFunc(keyFunc func(*model.Request) string) Option {
	return func(o *Options) {
		o.KeyFunc = keyFunc
	}
}

// DefaultOptions returns the default cache configuration.
func DefaultOptions() Options {
	return Options{
		SimilarityThreshold: 0.90,
		TTL:                 time.Hour,
		MaxSize:             1000,
		KeyFunc:             defaultKeyFunc,
	}
}

// defaultKeyFunc generates a cache key from the request messages and system prompt.
func defaultKeyFunc(req *model.Request) string {
	var key string
	for _, msg := range req.Messages {
		key += message.Stringify(msg) + "\n"
	}
	if req.SystemPrompt != "" {
		key += "SYSTEM: " + req.SystemPrompt + "\n"
	}
	return key
}

// ApplyOptions applies functional options to an Options struct.
func ApplyOptions(opts ...Option) Options {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}
	return options
}
