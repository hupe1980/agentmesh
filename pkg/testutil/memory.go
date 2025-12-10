package testutil

import (
	"context"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/memory"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MockMemory is a configurable mock implementation of memory.Memory.
type MockMemory struct {
	mu       sync.Mutex
	sessions map[string][]message.Message

	StoreFunc    func(ctx context.Context, sessionID string, messages []message.Message) error
	RecallFunc   func(ctx context.Context, sessionID string, filter memory.RecallFilter) ([]message.Message, error)
	ClearFunc    func(ctx context.Context, sessionID string) error
	SessionsFunc func(ctx context.Context) ([]string, error)
}

// NewMockMemory creates a new MockMemory with in-memory storage.
func NewMockMemory() *MockMemory {
	return &MockMemory{
		sessions: make(map[string][]message.Message),
	}
}

// Store persists messages for a given session.
func (m *MockMemory) Store(ctx context.Context, sessionID string, messages []message.Message) error {
	if m.StoreFunc != nil {
		return m.StoreFunc(ctx, sessionID, messages)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sessions == nil {
		m.sessions = make(map[string][]message.Message)
	}

	m.sessions[sessionID] = append(m.sessions[sessionID], messages...)
	return nil
}

// Recall retrieves messages for a session.
func (m *MockMemory) Recall(ctx context.Context, sessionID string, filter memory.RecallFilter) ([]message.Message, error) {
	if m.RecallFunc != nil {
		return m.RecallFunc(ctx, sessionID, filter)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	msgs, ok := m.sessions[sessionID]
	if !ok {
		return []message.Message{}, nil
	}

	k := filter.K
	if k <= 0 {
		k = 10
	}
	if k < len(msgs) {
		return msgs[len(msgs)-k:], nil
	}
	return msgs, nil
}

// Clear removes all messages for a session.
func (m *MockMemory) Clear(ctx context.Context, sessionID string) error {
	if m.ClearFunc != nil {
		return m.ClearFunc(ctx, sessionID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
	return nil
}

// Sessions returns all session IDs.
func (m *MockMemory) Sessions(ctx context.Context) ([]string, error) {
	if m.SessionsFunc != nil {
		return m.SessionsFunc(ctx)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetStoredMessages returns all stored messages for a session (for test assertions).
func (m *MockMemory) GetStoredMessages(sessionID string) []message.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msgs, ok := m.sessions[sessionID]; ok {
		result := make([]message.Message, len(msgs))
		copy(result, msgs)
		return result
	}
	return []message.Message{}
}

// MessageCount returns the total number of messages stored for a session.
func (m *MockMemory) MessageCount(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions[sessionID])
}

// Reset clears all sessions.
func (m *MockMemory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string][]message.Message)
}

// Ensure MockMemory implements memory.Memory
var _ memory.Memory = (*MockMemory)(nil)
