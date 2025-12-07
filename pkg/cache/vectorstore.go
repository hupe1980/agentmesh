package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
)

// Metadata keys for VectorStore documents.
const (
	metaKeyRequest   = "_cache_request"
	metaKeyResponse  = "_cache_response"
	metaKeyCacheKey  = "_cache_key"
	metaKeyTimestamp = "_cache_timestamp"
)

// VectorStore is a cache backend that uses a VectorStore for similarity search.
// This enables persistent, scalable caching with any VectorStore implementation
// (Pinecone, Qdrant, pgvector, etc.).
type VectorStore struct {
	store    vectorstore.VectorStore
	embedder embedding.Embedder
	options  Options

	// namespace isolates this cache's data in the store
	namespace string
}

// VectorStoreOption configures the VectorStore cache.
type VectorStoreOption func(*VectorStore)

// WithNamespace sets the namespace for cache isolation.
func WithNamespace(ns string) VectorStoreOption {
	return func(c *VectorStore) {
		c.namespace = ns
	}
}

// NewVectorStore creates a VectorStore-backed cache.
// The store is used for persistent vector similarity search.
// The embedder converts prompts to vectors.
func NewVectorStore(store vectorstore.VectorStore, embedder embedding.Embedder, opts ...any) *VectorStore {
	c := &VectorStore{
		store:     store,
		embedder:  embedder,
		options:   DefaultOptions(),
		namespace: "cache",
	}

	// Apply options
	var cacheOpts []Option
	var vsOpts []VectorStoreOption

	for _, opt := range opts {
		switch o := opt.(type) {
		case Option:
			cacheOpts = append(cacheOpts, o)
		case VectorStoreOption:
			vsOpts = append(vsOpts, o)
		}
	}

	c.options = ApplyOptions(cacheOpts...)
	for _, opt := range vsOpts {
		opt(c)
	}

	return c
}

// Get retrieves a cached response for a similar request.
func (c *VectorStore) Get(ctx context.Context, req *model.Request) (*model.Response, error) {
	// Generate cache key and embedding
	key := c.options.KeyFunc(req)

	queryEmb, err := c.embedder.Embed(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to embed request: %w", err)
	}

	// Search for similar entries
	docs, err := c.store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:         1, // Only need the best match
		MinScore:  c.options.SimilarityThreshold,
		Namespace: c.namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search cache: %w", err)
	}

	if len(docs) == 0 {
		return nil, nil // Cache miss
	}

	doc := docs[0]

	// Check TTL
	if c.options.TTL > 0 && time.Since(doc.Timestamp) > c.options.TTL {
		// Entry expired, delete it
		_ = c.store.Delete(ctx, []string{doc.ID}, c.namespace)
		return nil, nil // Cache miss
	}

	// Deserialize response
	resp, err := deserializeResponse(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize response: %w", err)
	}

	// Add cache hit metadata
	if resp.Metadata == nil {
		resp.Metadata = make(map[string]any)
	}
	resp.Metadata["cache_hit"] = true
	resp.Metadata["cache_similarity"] = doc.Score

	return resp, nil
}

// Set stores a response in the cache.
func (c *VectorStore) Set(ctx context.Context, req *model.Request, resp *model.Response) error {
	// Generate cache key and embedding
	key := c.options.KeyFunc(req)

	emb, err := c.embedder.Embed(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to embed request: %w", err)
	}

	// Serialize request and response
	reqData, err := serializeRequest(req)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	respData, err := serializeResponse(resp)
	if err != nil {
		return fmt.Errorf("failed to serialize response: %w", err)
	}

	// Create document
	doc := vectorstore.Document{
		ID:        uuid.New().String(),
		Content:   key,
		Embedding: emb,
		Timestamp: time.Now(),
		Metadata: vectorstore.Metadata{
			metaKeyCacheKey:  key,
			metaKeyRequest:   reqData,
			metaKeyResponse:  respData,
			metaKeyTimestamp: time.Now().Format(time.RFC3339Nano),
		},
	}

	// Store document
	return c.store.Add(ctx, []vectorstore.Document{doc}, func(o *vectorstore.AddOptions) {
		o.Namespace = c.namespace
	})
}

// Clear removes all entries from the cache.
func (c *VectorStore) Clear(ctx context.Context) error {
	// Search for all documents and delete them
	dims := c.embedder.Dimensions()
	zeroVec := make(embedding.Vector, dims)

	docs, err := c.store.Search(ctx, zeroVec, vectorstore.SearchOptions{
		Namespace: c.namespace,
		K:         10000,
		MinScore:  0,
	})
	if err != nil {
		return fmt.Errorf("failed to search for cache entries: %w", err)
	}

	if len(docs) > 0 {
		ids := make([]string, len(docs))
		for i, doc := range docs {
			ids[i] = doc.ID
		}
		if err := c.store.Delete(ctx, ids, c.namespace); err != nil {
			return fmt.Errorf("failed to delete cache entries: %w", err)
		}
	}

	return nil
}

// Close releases resources.
func (c *VectorStore) Close() error {
	return nil // Don't close the store - it's shared
}

// serialization helpers

// serializableResponse is a JSON-friendly version of model.Response.
type serializableResponse struct {
	Message      json.RawMessage `json:"message,omitempty"`
	Reasoning    string          `json:"reasoning,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
}

func serializeRequest(req *model.Request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func serializeResponse(resp *model.Response) (string, error) {
	// Serialize the message using the message package's serializer
	var msgData json.RawMessage
	if resp.Message != nil {
		var err error
		msgData, err = message.MarshalMessage(resp.Message)
		if err != nil {
			return "", fmt.Errorf("failed to serialize message: %w", err)
		}
	}

	sr := serializableResponse{
		Message:      msgData,
		Reasoning:    resp.Reasoning,
		FinishReason: resp.FinishReason,
		Metadata:     resp.Metadata,
	}

	data, err := json.Marshal(sr)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func deserializeResponse(meta vectorstore.Metadata) (*model.Response, error) {
	respData, ok := meta[metaKeyResponse].(string)
	if !ok {
		return nil, fmt.Errorf("missing response data")
	}

	var sr serializableResponse
	if err := json.Unmarshal([]byte(respData), &sr); err != nil {
		return nil, err
	}

	resp := &model.Response{
		Reasoning:    sr.Reasoning,
		FinishReason: sr.FinishReason,
		Metadata:     sr.Metadata,
	}

	// Deserialize the message using the message package's deserializer
	if len(sr.Message) > 0 {
		msg, err := message.UnmarshalMessage(sr.Message)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize message: %w", err)
		}
		resp.Message = msg
	}

	return resp, nil
}
