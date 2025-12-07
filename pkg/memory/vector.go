package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// VectorMemory implements semantic search over stored messages using vector embeddings.
type VectorMemory struct {
	embedder embedding.Embedder
	store    map[string][]*MessageEntry // sessionID -> entries
	mu       sync.RWMutex
}

// NewVectorMemory creates a new vector-based memory store.
func NewVectorMemory(embedder embedding.Embedder) *VectorMemory {
	return &VectorMemory{
		embedder: embedder,
		store:    make(map[string][]*MessageEntry),
	}
}

// Store persists messages with their vector embeddings.
func (vm *VectorMemory) Store(ctx context.Context, sessionID string, messages []message.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Extract text from messages for embedding
	texts := make([]string, len(messages))
	for i, msg := range messages {
		texts[i] = extractText(msg)
	}

	// Generate embeddings
	embeddings, err := vm.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Create entries with incrementing timestamps to preserve order within batch
	entries := make([]*MessageEntry, len(messages))
	baseTime := time.Now()
	for i, msg := range messages {
		entries[i] = &MessageEntry{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Message:   msg,
			Embedding: embeddings[i],
			Timestamp: baseTime.Add(time.Duration(i) * time.Nanosecond), // Preserve order within batch
			Metadata:  make(map[string]string),
		}
	}

	// Store entries
	vm.mu.Lock()
	vm.store[sessionID] = append(vm.store[sessionID], entries...)
	vm.mu.Unlock()

	return nil
}

// applySemanticSearch performs semantic search on candidates using query embedding.
func (vm *VectorMemory) applySemanticSearch(ctx context.Context, candidates []*MessageEntry, filter RecallFilter) ([]*MessageEntry, error) {
	queryEmbedding, err := vm.embedder.Embed(ctx, filter.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Calculate similarity scores
	for _, entry := range candidates {
		entry.Score = cosineSimilarity(queryEmbedding, entry.Embedding)
	}

	// Filter by minimum score
	if filter.MinScore > 0 {
		filtered := make([]*MessageEntry, 0, len(candidates))
		for _, entry := range candidates {
			if entry.Score >= filter.MinScore {
				filtered = append(filtered, entry)
			}
		}
		candidates = filtered
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates, nil
}

// Recall retrieves messages using semantic search or filters.
//
//nolint:gocyclo // Message filtering and retrieval requires multiple conditions
func (vm *VectorMemory) Recall(ctx context.Context, sessionID string, filter RecallFilter) ([]message.Message, error) {
	filter.Normalize()

	vm.mu.RLock()
	entries, exists := vm.store[sessionID]
	vm.mu.RUnlock()

	if !exists || len(entries) == 0 {
		return nil, nil
	}

	// Make a copy to work with
	candidates := make([]*MessageEntry, 0, len(entries))
	for _, entry := range entries {
		// Apply type filter
		if len(filter.Types) > 0 && !containsType(filter.Types, entry.Message.Type()) {
			continue
		}

		// Apply time filters
		if filter.After != nil && entry.Timestamp.Before(*filter.After) {
			continue
		}
		if filter.Before != nil && entry.Timestamp.After(*filter.Before) {
			continue
		}

		// Apply metadata filters
		if len(filter.Metadata) > 0 && !matchesMetadata(entry.Metadata, filter.Metadata) {
			continue
		}

		candidates = append(candidates, entry)
	}

	// If query provided, do semantic search
	if filter.Query != "" {
		var err error
		candidates, err = vm.applySemanticSearch(ctx, candidates, filter)
		if err != nil {
			return nil, err
		}
	} else {
		// No query: sort by timestamp descending (most recent first)
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Timestamp.After(candidates[j].Timestamp)
		})
	}

	// Limit to K results
	if len(candidates) > filter.K {
		candidates = candidates[:filter.K]
	}

	// Extract messages
	results := make([]message.Message, len(candidates))
	for i, entry := range candidates {
		results[i] = entry.Message
	}

	return results, nil
}

// Clear removes all messages for a session.
func (vm *VectorMemory) Clear(ctx context.Context, sessionID string) error {
	vm.mu.Lock()
	delete(vm.store, sessionID)
	vm.mu.Unlock()
	return nil
}

// Sessions returns all session IDs.
func (vm *VectorMemory) Sessions(ctx context.Context) ([]string, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	sessions := make([]string, 0, len(vm.store))
	for sessionID := range vm.store {
		sessions = append(sessions, sessionID)
	}
	return sessions, nil
}

// Helper functions

func extractText(msg message.Message) string {
	parts := msg.Parts()
	if len(parts) == 0 {
		return ""
	}

	var text string
	for _, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			if text != "" {
				text += " "
			}
			text += textPart.Text
		}
	}
	return text
}

func containsType(types []message.Type, t message.Type) bool {
	for _, typ := range types {
		if typ == t {
			return true
		}
	}
	return false
}

func matchesMetadata(entryMeta, filterMeta map[string]string) bool {
	for key, value := range filterMeta {
		if entryMeta[key] != value {
			return false
		}
	}
	return true
}

func cosineSimilarity(a, b []float64) float64 {
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
