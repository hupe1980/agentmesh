package openai

import (
	"context"
	"testing"

	gopenai "github.com/openai/openai-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockEmbeddingClient is a mock implementation of EmbeddingClient for testing.
type MockEmbeddingClient struct {
	CreateEmbeddingFunc func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error)
}

func (m *MockEmbeddingClient) CreateEmbedding(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
	if m.CreateEmbeddingFunc != nil {
		return m.CreateEmbeddingFunc(ctx, params)
	}
	return nil, nil
}

func TestEmbedderDimensions(t *testing.T) {
	t.Run("Default dimensions", func(t *testing.T) {
		embedder := NewEmbedder(func(o *Options) {
			o.Model = "text-embedding-3-small"
		})
		assert.Equal(t, 1536, embedder.Dimensions())
	})

	t.Run("Custom dimensions", func(t *testing.T) {
		embedder := NewEmbedder(func(o *Options) {
			o.Model = "text-embedding-3-small"
			o.Dimensions = 512
		})
		assert.Equal(t, 512, embedder.Dimensions())
	})

	t.Run("text-embedding-3-large default", func(t *testing.T) {
		embedder := NewEmbedder(func(o *Options) {
			o.Model = "text-embedding-3-large"
		})
		assert.Equal(t, 3072, embedder.Dimensions())
	})

	t.Run("text-embedding-ada-002 default", func(t *testing.T) {
		embedder := NewEmbedder(func(o *Options) {
			o.Model = "text-embedding-ada-002"
		})
		assert.Equal(t, 1536, embedder.Dimensions())
	})
}

func TestEmbedderWithMock(t *testing.T) {
	ctx := context.Background()

	t.Run("Embed single text", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				// Verify correct parameters
				assert.NotNil(t, params.Input.OfString)
				assert.Equal(t, "Hello, world!", params.Input.OfString.Value)

				return &gopenai.CreateEmbeddingResponse{
					Data: []gopenai.Embedding{
						{Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5}},
					},
				}, nil
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		embedding, err := embedder.Embed(ctx, "Hello, world!")
		require.NoError(t, err)
		assert.Len(t, embedding, 5)
		assert.Equal(t, []float32{0.1, 0.2, 0.3, 0.4, 0.5}, embedding)
	})

	t.Run("Embed empty text fails", func(t *testing.T) {
		embedder := &Embedder{
			client: &MockEmbeddingClient{},
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.Embed(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty text")
	})

	t.Run("Embed returns no embeddings error", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				return &gopenai.CreateEmbeddingResponse{
					Data: []gopenai.Embedding{}, // Empty response
				}, nil
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.Embed(ctx, "test text")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no embeddings returned")
	})

	t.Run("Embed with custom dimensions", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				// Verify dimensions parameter is set
				assert.NotNil(t, params.Dimensions)
				assert.Equal(t, int64(512), params.Dimensions.Value)

				return &gopenai.CreateEmbeddingResponse{
					Data: []gopenai.Embedding{
						{Embedding: make([]float64, 512)},
					},
				}, nil
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   512,
		}

		embedding, err := embedder.Embed(ctx, "test")
		require.NoError(t, err)
		assert.Len(t, embedding, 512)
	})

	t.Run("EmbedBatch multiple texts", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				// Verify array of strings
				assert.Len(t, params.Input.OfArrayOfStrings, 3)
				assert.Equal(t, []string{"Hello", "World", "Test"}, params.Input.OfArrayOfStrings)

				return &gopenai.CreateEmbeddingResponse{
					Data: []gopenai.Embedding{
						{Embedding: []float64{0.1, 0.2, 0.3}},
						{Embedding: []float64{0.4, 0.5, 0.6}},
						{Embedding: []float64{0.7, 0.8, 0.9}},
					},
				}, nil
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		embeddings, err := embedder.EmbedBatch(ctx, []string{"Hello", "World", "Test"})
		require.NoError(t, err)
		assert.Len(t, embeddings, 3)
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, embeddings[0])
		assert.Equal(t, []float32{0.4, 0.5, 0.6}, embeddings[1])
		assert.Equal(t, []float32{0.7, 0.8, 0.9}, embeddings[2])
	})

	t.Run("EmbedBatch filters empty strings", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				// Should only receive non-empty strings
				assert.Len(t, params.Input.OfArrayOfStrings, 2)
				assert.Equal(t, []string{"Hello", "World"}, params.Input.OfArrayOfStrings)

				return &gopenai.CreateEmbeddingResponse{
					Data: []gopenai.Embedding{
						{Embedding: []float64{0.1, 0.2, 0.3}},
						{Embedding: []float64{0.4, 0.5, 0.6}},
					},
				}, nil
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		embeddings, err := embedder.EmbedBatch(ctx, []string{"Hello", "", "World"})
		require.NoError(t, err)
		assert.Len(t, embeddings, 2) // Only non-empty strings
	})

	t.Run("EmbedBatch empty slice fails", func(t *testing.T) {
		embedder := &Embedder{
			client: &MockEmbeddingClient{},
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.EmbedBatch(ctx, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty texts")
	})

	t.Run("EmbedBatch all empty fails", func(t *testing.T) {
		embedder := &Embedder{
			client: &MockEmbeddingClient{},
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.EmbedBatch(ctx, []string{"", ""})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "all texts are empty")
	})

	t.Run("EmbedBatch wrong count error", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				// Return wrong number of embeddings
				return &gopenai.CreateEmbeddingResponse{
					Data: []gopenai.Embedding{
						{Embedding: []float64{0.1, 0.2, 0.3}},
					},
				}, nil
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.EmbedBatch(ctx, []string{"text1", "text2"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected 2 embeddings, got 1")
	})

	t.Run("Embed handles API error", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				return nil, assert.AnError
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.Embed(ctx, "test text")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create embedding")
	})

	t.Run("EmbedBatch handles API error", func(t *testing.T) {
		mockClient := &MockEmbeddingClient{
			CreateEmbeddingFunc: func(ctx context.Context, params gopenai.EmbeddingNewParams) (*gopenai.CreateEmbeddingResponse, error) {
				return nil, assert.AnError
			},
		}

		embedder := &Embedder{
			client: mockClient,
			model:  "text-embedding-3-small",
			dims:   0,
		}

		_, err := embedder.EmbedBatch(ctx, []string{"text1", "text2"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create batch embeddings")
	})
}
