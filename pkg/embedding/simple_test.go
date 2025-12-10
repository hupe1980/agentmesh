package embedding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleEmbedder_Dimensions(t *testing.T) {
	t.Run("Default dimensions", func(t *testing.T) {
		embedder := NewSimpleEmbedder(0)
		assert.Equal(t, 128, embedder.Dimensions())
	})

	t.Run("Custom dimensions", func(t *testing.T) {
		embedder := NewSimpleEmbedder(256)
		assert.Equal(t, 256, embedder.Dimensions())
	})

	t.Run("Negative dimensions defaults to 128", func(t *testing.T) {
		embedder := NewSimpleEmbedder(-10)
		assert.Equal(t, 128, embedder.Dimensions())
	})
}

func TestSimpleEmbedder_Embed(t *testing.T) {
	ctx := context.Background()
	embedder := NewSimpleEmbedder(128)

	t.Run("Non-empty text", func(t *testing.T) {
		embedding, err := embedder.Embed(ctx, "Hello, world!")
		require.NoError(t, err)
		assert.Len(t, embedding, 128)

		// Check that embedding is normalized (unit length)
		var sum float32
		for _, v := range embedding {
			sum += v * v
		}
		assert.InDelta(t, 1.0, sum, 0.0001, "Embedding should be normalized")
	})

	t.Run("Empty text", func(t *testing.T) {
		embedding, err := embedder.Embed(ctx, "")
		require.NoError(t, err)
		assert.Len(t, embedding, 128)

		// All zeros for empty text
		for _, v := range embedding {
			assert.Equal(t, float32(0.0), v)
		}
	})

	t.Run("Deterministic", func(t *testing.T) {
		text := "Test text for determinism"
		embedding1, err1 := embedder.Embed(ctx, text)
		embedding2, err2 := embedder.Embed(ctx, text)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, embedding1, embedding2, "Same text should produce same embedding")
	})

	t.Run("Different texts produce different embeddings", func(t *testing.T) {
		embedding1, err1 := embedder.Embed(ctx, "Hello")
		embedding2, err2 := embedder.Embed(ctx, "World")

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, embedding1, embedding2)
	})
}

func TestSimpleEmbedder_EmbedBatch(t *testing.T) {
	ctx := context.Background()
	embedder := NewSimpleEmbedder(128)

	t.Run("Multiple texts", func(t *testing.T) {
		texts := []string{"Hello", "World", "Test"}
		embeddings, err := embedder.EmbedBatch(ctx, texts)

		require.NoError(t, err)
		assert.Len(t, embeddings, 3)

		for i, embedding := range embeddings {
			assert.Len(t, embedding, 128)

			// Verify each embedding is normalized
			var sum float32
			for _, v := range embedding {
				sum += v * v
			}
			assert.InDelta(t, 1.0, sum, 0.0001, "Embedding %d should be normalized", i)
		}
	})

	t.Run("Empty slice", func(t *testing.T) {
		embeddings, err := embedder.EmbedBatch(ctx, []string{})
		require.NoError(t, err)
		assert.Empty(t, embeddings)
	})

	t.Run("Batch produces same results as individual", func(t *testing.T) {
		texts := []string{"Hello", "World"}

		// Individual embeds
		embedding1, err1 := embedder.Embed(ctx, texts[0])
		embedding2, err2 := embedder.Embed(ctx, texts[1])
		require.NoError(t, err1)
		require.NoError(t, err2)

		// Batch embed
		embeddings, err := embedder.EmbedBatch(ctx, texts)
		require.NoError(t, err)

		assert.Equal(t, embedding1, embeddings[0])
		assert.Equal(t, embedding2, embeddings[1])
	})
}

func TestNormalizeSimple(t *testing.T) {
	t.Run("Normalize non-zero vector", func(t *testing.T) {
		vec := []float32{3.0, 4.0}
		normalized := normalizeSimple(vec)

		// Length should be 1
		var sum float32
		for _, v := range normalized {
			sum += v * v
		}
		assert.InDelta(t, 1.0, sum, 0.0001)

		// Values should be 3/5 and 4/5
		assert.InDelta(t, 0.6, normalized[0], 0.0001)
		assert.InDelta(t, 0.8, normalized[1], 0.0001)
	})

	t.Run("Normalize zero vector", func(t *testing.T) {
		vec := []float32{0.0, 0.0, 0.0}
		normalized := normalizeSimple(vec)

		// Should return the same vector
		assert.Equal(t, vec, normalized)
	})
}
