package cache

import (
	"github.com/hupe1980/agentmesh/pkg/embedding"
)

// FindMostSimilar finds the entry with highest similarity above the threshold.
// Returns the entry and its similarity score, or nil if no match found.
func FindMostSimilar(queryEmbedding []float64, entries []*Entry, threshold float64) (*Entry, float64) {
	var bestEntry *Entry
	var bestScore float64

	for _, entry := range entries {
		score := embedding.CosineSimilarity(queryEmbedding, entry.Embedding)
		if score >= threshold && score > bestScore {
			bestEntry = entry
			bestScore = score
		}
	}

	return bestEntry, bestScore
}
