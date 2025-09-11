package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertions)
var _ core.MemoryStore = (*InMemoryStore)(nil)

// userKey generates a unique key for a user in an app.
func userKey(appName, userID string) string {
	return appName + ":" + userID
}

// InMemoryStore is a naive process‑local MemoryStore. It offers:
//  1. Session scoped key/value memory (Get / Put)
//  2. Append‑only stored memories with substring Search
//
// Concurrency: protected by RWMutex.
// Search: linear scan with substring matching (case sensitive) assigning a
// constant score of 1.0 to every hit. Suitable only for tests / demos; swap for
// a vector DB or semantic index for production retrieval.
type InMemoryStore struct {
	sessionEvents map[string]map[string][]*core.Event // userKey -> sessionID -> []*Event
	// sessionOrder keeps sessionIDs per userKey in insertion order to enable
	// deterministic, append-order iteration.
	sessionOrder map[string][]string // userKey -> []sessionID
	mu           sync.RWMutex
}

// NewInMemoryStore creates a new in-memory memory store
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessionEvents: make(map[string]map[string][]*core.Event),
		sessionOrder:  make(map[string][]string),
	}
}

// AddSession stores filtered events for the session.
func (m *InMemoryStore) AddSession(_ context.Context, session *core.Session) error {
	userKey := userKey(session.AppName(), session.UserID())
	sessionID := session.ID()

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessionEvents[userKey]; !ok {
		m.sessionEvents[userKey] = make(map[string][]*core.Event)
	}

	events := session.Events()
	filtered := make([]*core.Event, 0, len(events))
	for _, event := range events {
		if event != nil && len(event.Parts) > 0 {
			// Store a clone to prevent external mutation of internal state
			filtered = append(filtered, event.Clone())
		}
	}

	// If this is a new session for this user, remember insertion order
	if _, exists := m.sessionEvents[userKey][sessionID]; !exists {
		m.sessionOrder[userKey] = append(m.sessionOrder[userKey], sessionID)
	}

	m.sessionEvents[userKey][sessionID] = filtered

	return nil
}

// Search returns events matching the query for the given appName and userID.
func (m *InMemoryStore) Search(_ context.Context, appName, userID, query string) (*core.SearchResult, error) {
	userKey := userKey(appName, userID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionMap, exists := m.sessionEvents[userKey]
	if !exists {
		return &core.SearchResult{Memories: nil}, nil
	}

	// Iterate sessions strictly in insertion order; error if order is missing
	ids, ok := m.sessionOrder[userKey]
	if !ok || len(ids) == 0 {
		return nil, core.ErrMemoryNotFound
	}

	// Aggregate all matching events as MemoryItems
	var items []*core.MemoryItem
	for _, id := range ids {
		events := sessionMap[id]
		for _, event := range events {
			if event == nil {
				continue
			}

			if query == "" || partsContains(event.Parts, query) {
				// Return cloned parts to avoid callers mutating stored content
				var clonedParts []core.Part
				if len(event.Parts) > 0 {
					clonedParts = make([]core.Part, len(event.Parts))
					for i, p := range event.Parts {
						clonedParts[i] = core.ClonePart(p)
					}
				}

				items = append(items, &core.MemoryItem{
					Parts:     clonedParts,
					Author:    event.Author,
					Timestamp: event.Timestamp,
				})
			}
		}
	}

	return &core.SearchResult{Memories: items}, nil
}

// partsContains checks whether any TextPart within parts contains the query (case-insensitive).
func partsContains(parts []core.Part, query string) bool {
	q := strings.ToLower(query)
	for _, p := range parts {
		if tp, ok := p.(*core.TextPart); ok {
			if strings.Contains(strings.ToLower(tp.Text), q) {
				return true
			}
		}
	}

	return false
}

// Close implements core.MemoryStore. No resources to release for in-memory store.
func (m *InMemoryStore) Close() error { return nil }
