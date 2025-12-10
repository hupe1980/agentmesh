package openai

import (
	"context"
	"errors"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/floatconv"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/param"
)

// EmbeddingClient defines the interface for interacting with the OpenAI Embeddings API.
type EmbeddingClient interface {
	CreateEmbedding(ctx context.Context, params openai.EmbeddingNewParams) (*openai.CreateEmbeddingResponse, error)
}

// EmbeddingClientWrapper wraps an OpenAI client to implement the EmbeddingClient interface.
type EmbeddingClientWrapper struct {
	inner *openai.Client
}

// NewEmbeddingClientWrapper creates a new EmbeddingClientWrapper.
func NewEmbeddingClientWrapper(client *openai.Client) *EmbeddingClientWrapper {
	return &EmbeddingClientWrapper{
		inner: client,
	}
}

// CreateEmbedding implements the CreateEmbedding method of the EmbeddingClient interface.
func (c *EmbeddingClientWrapper) CreateEmbedding(
	ctx context.Context,
	params openai.EmbeddingNewParams,
) (*openai.CreateEmbeddingResponse, error) {
	return c.inner.Embeddings.New(ctx, params)
}

// Embedder implements the embedding.Embedder interface using OpenAI's embedding API.
type Embedder struct {
	client EmbeddingClient
	model  string
	dims   int
}

// Options configures the OpenAI embedder.
type Options struct {
	// Model specifies which embedding model to use.
	// Default: "text-embedding-3-small"
	Model string

	// Dimensions specifies the output embedding dimensions (only for text-embedding-3-* models).
	// Set to 0 to use the model's default dimensions.
	// Default: 0 (use model default)
	Dimensions int
}

// NewEmbedder creates a new OpenAI embedder with the specified configuration.
//
// Example:
//
//	embedder := openai.NewEmbedder(func(o *openai.Options) {
//	    o.Model = "text-embedding-3-large"
//	    o.Dimensions = 1024
//	})
func NewEmbedder(optFns ...func(*Options)) embedding.Embedder {
	client := openai.NewClient()
	return NewEmbedderFromClient(&client, optFns...)
}

// NewEmbedderFromClient creates a new OpenAI embedder with a custom client.
// This is useful for testing or when you need custom client configuration.
func NewEmbedderFromClient(client *openai.Client, optFns ...func(*Options)) embedding.Embedder {
	return NewEmbedderFromClientWrapper(NewEmbeddingClientWrapper(client), optFns...)
}

// NewEmbedderFromClientWrapper creates a new OpenAI embedder with a custom client wrapper.
// This is useful for testing with mock clients.
func NewEmbedderFromClientWrapper(wrapper *EmbeddingClientWrapper, optFns ...func(*Options)) embedding.Embedder {
	opts := &Options{
		Model:      "text-embedding-3-small",
		Dimensions: 0,
	}

	for _, fn := range optFns {
		fn(opts)
	}

	return &Embedder{
		client: wrapper,
		model:  opts.Model,
		dims:   opts.Dimensions,
	}
}

// Embed converts text into a vector embedding using OpenAI's API.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, errors.New("openai embedder: empty text provided")
	}

	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		Model: e.model,
	}

	// Set dimensions if specified (only works for text-embedding-3-* models)
	if e.dims > 0 {
		params.Dimensions = param.NewOpt(int64(e.dims))
	}

	resp, err := e.client.CreateEmbedding(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, errors.New("openai embedder: no embeddings returned")
	}

	// Convert from float64 (SDK default) to float32 (no precision loss, API data is float32)
	return floatconv.ToFloat32(resp.Data[0].Embedding), nil
}

// EmbedBatch converts multiple texts into vector embeddings efficiently using a single API call.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errors.New("openai embedder: empty texts slice provided")
	}

	// Filter out empty strings
	validTexts := make([]string, 0, len(texts))
	for _, text := range texts {
		if text != "" {
			validTexts = append(validTexts, text)
		}
	}

	if len(validTexts) == 0 {
		return nil, errors.New("openai embedder: all texts are empty")
	}

	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: validTexts,
		},
		Model: e.model,
	}

	// Set dimensions if specified
	if e.dims > 0 {
		params.Dimensions = param.NewOpt(int64(e.dims))
	}

	resp, err := e.client.CreateEmbedding(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: failed to create batch embeddings: %w", err)
	}

	if len(resp.Data) != len(validTexts) {
		return nil, fmt.Errorf("openai embedder: expected %d embeddings, got %d", len(validTexts), len(resp.Data))
	}

	// Extract embeddings in order, converting to float32
	embeddings := make([][]float32, len(resp.Data))
	for i := range resp.Data {
		embeddings[i] = floatconv.ToFloat32(resp.Data[i].Embedding)
	}

	return embeddings, nil
}

// Dimensions returns the dimensionality of the embedding vectors.
// If dimensions were explicitly set, returns that value.
// Otherwise, returns the default dimensions for the model.
func (e *Embedder) Dimensions() int {
	if e.dims > 0 {
		return e.dims
	}

	// Return default dimensions based on model
	switch e.model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	default:
		return 1536 // Fallback to most common dimension
	}
}
