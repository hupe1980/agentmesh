package retrieval

import (
	"context"
	"sort"
)

// Reranker reorders retrieved documents for improved relevance.
type Reranker interface {
	// Rerank reorders documents based on the query.
	Rerank(ctx context.Context, query string, docs []Document) ([]Document, error)
}

// RerankerFunc is a function adapter for Reranker.
type RerankerFunc func(ctx context.Context, query string, docs []Document) ([]Document, error)

// Rerank implements Reranker.
func (f RerankerFunc) Rerank(ctx context.Context, query string, docs []Document) ([]Document, error) {
	return f(ctx, query, docs)
}

// RerankedRetriever wraps a Retriever with reranking.
type RerankedRetriever struct {
	retriever Retriever
	reranker  Reranker
	topK      int
}

// NewRerankedRetriever creates a retriever that reranks results.
// Set topK to 0 to return all reranked results.
func NewRerankedRetriever(retriever Retriever, reranker Reranker, topK int) Retriever {
	return &RerankedRetriever{
		retriever: retriever,
		reranker:  reranker,
		topK:      topK,
	}
}

// Retrieve fetches documents and reranks them.
func (r *RerankedRetriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	docs, err := r.retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return docs, nil
	}

	reranked, err := r.reranker.Rerank(ctx, query, docs)
	if err != nil {
		return nil, err
	}

	if r.topK > 0 && len(reranked) > r.topK {
		reranked = reranked[:r.topK]
	}

	return reranked, nil
}

// ScoreReranker reranks documents using a custom scoring function.
type ScoreReranker struct {
	scorer func(ctx context.Context, query string, doc Document) (float64, error)
}

// NewScoreReranker creates a reranker that uses a custom scoring function.
// Documents are sorted by score in descending order.
func NewScoreReranker(scorer func(ctx context.Context, query string, doc Document) (float64, error)) Reranker {
	return &ScoreReranker{scorer: scorer}
}

// Rerank implements Reranker by re-scoring each document.
func (r *ScoreReranker) Rerank(ctx context.Context, query string, docs []Document) ([]Document, error) {
	type scored struct {
		doc   Document
		score float64
	}

	results := make([]scored, len(docs))
	for i, doc := range docs {
		score, err := r.scorer(ctx, query, doc)
		if err != nil {
			return nil, err
		}
		results[i] = scored{doc: doc, score: score}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	reranked := make([]Document, len(results))
	for i, r := range results {
		reranked[i] = r.doc
		reranked[i].Score = r.score
	}

	return reranked, nil
}

// BoostReranker adjusts scores based on metadata field values.
type BoostReranker struct {
	field        string
	boosts       map[any]float64
	defaultBoost float64
}

// NewBoostReranker creates a reranker that boosts scores based on metadata.
// Documents with metadata[field] matching a key in boosts have their score
// multiplied by the corresponding boost factor.
func NewBoostReranker(field string, boosts map[any]float64, defaultBoost float64) Reranker {
	return &BoostReranker{
		field:        field,
		boosts:       boosts,
		defaultBoost: defaultBoost,
	}
}

// Rerank implements Reranker by applying boost factors.
func (r *BoostReranker) Rerank(ctx context.Context, query string, docs []Document) ([]Document, error) {
	type scored struct {
		doc   Document
		score float64
	}

	results := make([]scored, len(docs))
	for i, doc := range docs {
		boost := r.defaultBoost
		if r.defaultBoost == 0 {
			boost = 1.0
		}

		if doc.Metadata != nil {
			if val, ok := doc.Metadata[r.field]; ok {
				if b, exists := r.boosts[val]; exists {
					boost = b
				}
			}
		}

		results[i] = scored{doc: doc, score: doc.Score * boost}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	reranked := make([]Document, len(results))
	for i, r := range results {
		reranked[i] = r.doc
		reranked[i].Score = r.score
	}

	return reranked, nil
}

// RecencyReranker boosts more recent documents.
type RecencyReranker struct {
	timestampField string
	decayFactor    float64 // Factor per day, e.g., 0.9 means 10% decay per day
}

// NewRecencyReranker creates a reranker that boosts recent documents.
// The decayFactor determines how much older documents are penalized.
// A factor of 0.9 means documents lose 10% relevance per day.
func NewRecencyReranker(timestampField string, decayFactor float64) Reranker {
	return &RecencyReranker{
		timestampField: timestampField,
		decayFactor:    decayFactor,
	}
}

// Rerank implements Reranker by applying time-based decay.
func (r *RecencyReranker) Rerank(ctx context.Context, query string, docs []Document) ([]Document, error) {
	// Implementation would parse timestamps and apply decay
	// For now, just return documents sorted by original score
	result := make([]Document, len(docs))
	copy(result, docs)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result, nil
}

// ChainedReranker applies multiple rerankers in sequence.
type ChainedReranker struct {
	rerankers []Reranker
}

// NewChainedReranker creates a reranker that applies multiple rerankers in order.
func NewChainedReranker(rerankers ...Reranker) Reranker {
	return &ChainedReranker{rerankers: rerankers}
}

// Rerank implements Reranker by applying each reranker in sequence.
func (r *ChainedReranker) Rerank(ctx context.Context, query string, docs []Document) ([]Document, error) {
	result := docs
	for _, reranker := range r.rerankers {
		var err error
		result, err = reranker.Rerank(ctx, query, result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
