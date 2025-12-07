package embedding

import "math"

// Metric specifies the distance/similarity metric for vector comparisons.
type Metric int

const (
	// Cosine measures the cosine of the angle between vectors.
	// Range: [-1, 1] where 1 is identical, 0 is orthogonal, -1 is opposite.
	Cosine Metric = iota

	// Euclidean measures the straight-line distance between vectors.
	// Range: [0, ∞) where 0 is identical. Converted to similarity as 1/(1+d).
	Euclidean

	// DotProduct measures the dot product of two vectors.
	// Best used with normalized vectors. Range depends on vector magnitudes.
	DotProduct
)

// String returns the string representation of the metric.
func (m Metric) String() string {
	switch m {
	case Cosine:
		return "cosine"
	case Euclidean:
		return "euclidean"
	case DotProduct:
		return "dot_product"
	default:
		return "unknown"
	}
}

// Similarity computes similarity between two vectors using the specified metric.
// Returns a value where higher means more similar.
func Similarity(a, b Vector, metric Metric) float64 {
	switch metric {
	case Cosine:
		return CosineSimilarity(a, b)
	case Euclidean:
		return 1 / (1 + EuclideanDistance(a, b))
	case DotProduct:
		return DotProductSimilarity(a, b)
	default:
		return CosineSimilarity(a, b)
	}
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [-1, 1] where 1 means identical direction.
func CosineSimilarity(a, b Vector) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// EuclideanDistance computes the Euclidean (L2) distance between two vectors.
// Returns 0 for identical vectors, higher values for more distant vectors.
func EuclideanDistance(a, b Vector) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return math.MaxFloat64
	}

	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// DotProductSimilarity computes the dot product of two vectors.
// Best used with normalized vectors for meaningful similarity scores.
func DotProductSimilarity(a, b Vector) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}

	return sum
}

// Normalize converts a vector to unit length (L2 normalization).
// Returns the original vector if it has zero magnitude.
func Normalize(v Vector) Vector {
	var sum float64
	for _, val := range v {
		sum += val * val
	}

	if sum == 0 {
		return v
	}

	norm := math.Sqrt(sum)
	result := make(Vector, len(v))
	for i, val := range v {
		result[i] = val / norm
	}

	return result
}

// Magnitude computes the L2 norm (length) of a vector.
func Magnitude(v Vector) float64 {
	var sum float64
	for _, val := range v {
		sum += val * val
	}
	return math.Sqrt(sum)
}
