package embedding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        Vector
		b        Vector
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "similar vectors",
			a:        Normalize([]float64{1, 1, 0}),
			b:        Normalize([]float64{1, 0, 0}),
			expected: 1 / math.Sqrt(2), // cos(45°) ≈ 0.707
		},
		{
			name:     "empty vectors",
			a:        []float64{},
			b:        []float64{},
			expected: 0,
		},
		{
			name:     "different lengths",
			a:        []float64{1, 0},
			b:        []float64{1, 0, 0},
			expected: 0,
		},
		{
			name:     "zero vector",
			a:        []float64{0, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        Vector
		b        Vector
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 0,
		},
		{
			name:     "unit distance",
			a:        []float64{0, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "diagonal",
			a:        []float64{0, 0, 0},
			b:        []float64{1, 1, 1},
			expected: math.Sqrt(3),
		},
		{
			name:     "empty vectors",
			a:        []float64{},
			b:        []float64{},
			expected: math.MaxFloat64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EuclideanDistance(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestDotProductSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        Vector
		b        Vector
		expected float64
	}{
		{
			name:     "unit vectors same direction",
			a:        []float64{1, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal",
			a:        []float64{1, 0, 0},
			b:        []float64{0, 1, 0},
			expected: 0,
		},
		{
			name:     "scaled vectors",
			a:        []float64{2, 0, 0},
			b:        []float64{3, 0, 0},
			expected: 6.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DotProductSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestNormalize_Similarity(t *testing.T) {
	t.Run("normalizes to unit length", func(t *testing.T) {
		v := []float64{3, 4, 0}
		normalized := Normalize(v)
		mag := Magnitude(normalized)
		assert.InDelta(t, 1.0, mag, 0.0001)
	})

	t.Run("zero vector unchanged", func(t *testing.T) {
		v := []float64{0, 0, 0}
		normalized := Normalize(v)
		assert.Equal(t, v, normalized)
	})
}

func TestMagnitude(t *testing.T) {
	tests := []struct {
		name     string
		v        Vector
		expected float64
	}{
		{
			name:     "unit vector",
			v:        []float64{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "3-4-5 triangle",
			v:        []float64{3, 4, 0},
			expected: 5.0,
		},
		{
			name:     "zero vector",
			v:        []float64{0, 0, 0},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Magnitude(tt.v)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestSimilarity(t *testing.T) {
	a := Normalize([]float64{1, 0, 0})
	b := Normalize([]float64{1, 0, 0})

	t.Run("cosine metric", func(t *testing.T) {
		result := Similarity(a, b, Cosine)
		assert.InDelta(t, 1.0, result, 0.0001)
	})

	t.Run("euclidean metric", func(t *testing.T) {
		result := Similarity(a, b, Euclidean)
		assert.InDelta(t, 1.0, result, 0.0001) // 1 / (1 + 0) = 1
	})

	t.Run("dot product metric", func(t *testing.T) {
		result := Similarity(a, b, DotProduct)
		assert.InDelta(t, 1.0, result, 0.0001)
	})
}

func TestMetricString(t *testing.T) {
	assert.Equal(t, "cosine", Cosine.String())
	assert.Equal(t, "euclidean", Euclidean.String())
	assert.Equal(t, "dot_product", DotProduct.String())
	assert.Equal(t, "unknown", Metric(99).String())
}
