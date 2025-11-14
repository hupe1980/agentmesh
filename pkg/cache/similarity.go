package cache

import (
	"math"
)

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
// For normalized vectors (embeddings), this is equivalent to dot product.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// FindMostSimilar finds the entry with highest similarity above the threshold.
// Returns the entry and its similarity score, or nil if no match found.
func FindMostSimilar(embedding []float64, entries []*Entry, threshold float64) (*Entry, float64) {
	var bestEntry *Entry
	var bestScore float64

	for _, entry := range entries {
		score := CosineSimilarity(embedding, entry.Embedding)
		if score >= threshold && score > bestScore {
			bestEntry = entry
			bestScore = score
		}
	}

	return bestEntry, bestScore
}
