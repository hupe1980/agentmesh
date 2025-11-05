package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// SimpleMemory implements basic FIFO message storage without semantic search.
// It's useful for simple conversation history without embedding overhead.
type SimpleMemory struct {
	store   map[string][]*MessageEntry
	maxSize int // Maximum messages per session (0 = unlimited)
	mu      sync.RWMutex
}

// NewSimpleMemory creates a new simple memory store.
// maxSize limits the number of messages per session (0 for unlimited).
func NewSimpleMemory(maxSize int) *SimpleMemory {
	return &SimpleMemory{
		store:   make(map[string][]*MessageEntry),
		maxSize: maxSize,
	}
}

// Store persists messages without embeddings.
func (sm *SimpleMemory) Store(ctx context.Context, sessionID string, messages []message.Message) error {
	if len(messages) == 0 {
		return nil
	}

	entries := make([]*MessageEntry, len(messages))
	now := time.Now()
	for i, msg := range messages {
		entries[i] = &MessageEntry{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Message:   msg,
			Timestamp: now,
			Metadata:  make(map[string]string),
		}
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.store[sessionID] = append(sm.store[sessionID], entries...)

	// Apply size limit (FIFO)
	if sm.maxSize > 0 && len(sm.store[sessionID]) > sm.maxSize {
		excess := len(sm.store[sessionID]) - sm.maxSize
		sm.store[sessionID] = sm.store[sessionID][excess:]
	}

	return nil
}

// Recall retrieves messages based on filters (without semantic search).
func (sm *SimpleMemory) Recall(ctx context.Context, sessionID string, filter RecallFilter) ([]message.Message, error) {
	filter.Normalize()

	sm.mu.RLock()
	entries, exists := sm.store[sessionID]
	sm.mu.RUnlock()

	if !exists || len(entries) == 0 {
		return nil, nil
	}

	// Filter entries
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

	// Sort by timestamp descending (most recent first)
	// Already in order for FIFO, but reverse to get most recent
	for i, j := 0, len(candidates)-1; i < j; i, j = i+1, j-1 {
		candidates[i], candidates[j] = candidates[j], candidates[i]
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
func (sm *SimpleMemory) Clear(ctx context.Context, sessionID string) error {
	sm.mu.Lock()
	delete(sm.store, sessionID)
	sm.mu.Unlock()
	return nil
}

// Sessions returns all session IDs.
func (sm *SimpleMemory) Sessions(ctx context.Context) ([]string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]string, 0, len(sm.store))
	for sessionID := range sm.store {
		sessions = append(sessions, sessionID)
	}
	return sessions, nil
}
