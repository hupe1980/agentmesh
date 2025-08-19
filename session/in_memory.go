package session

import (
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// InMemoryStore is a volatile SessionStore implementation storing
// sessions in a process local map. It is safe for concurrent access and best
// suited for tests or ephemeral demo servers. Each returned session is cloned
// to prevent external mutation of internal state.
type InMemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*core.Session
}

// NewInMemoryStore constructs an empty in‑memory session store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{sessions: make(map[string]*core.Session)}
}

// Get returns an existing session (clone) or creates a new one lazily.
func (s *InMemoryStore) Get(sessionID string) (*core.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if session, ok := s.sessions[sessionID]; ok {
		return session.Clone(), nil
	}
	return s.createSessionLocked(sessionID), nil
}

// SaveSession stores a clone of the provided session snapshot.
// Deprecated: retained temporarily for tests that still invoke SaveSession directly.
func (s *InMemoryStore) SaveSession(session *core.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session.Clone()
	return nil
}

// Create forces the creation (or overwriting) of a session with the given id.
func (s *InMemoryStore) Create(sessionID string) (*core.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createSessionLocked(sessionID).Clone(), nil
}

// AppendEvent adds an event to an existing or newly created session.
func (s *InMemoryStore) AppendEvent(sessionID string, ev core.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		sess = s.createSessionLocked(sessionID)
	}
	sess.AddEvent(ev)
	return nil
}

// ApplyDelta merges a key/value delta into the session state.
func (s *InMemoryStore) ApplyDelta(sessionID string, delta map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		sess = s.createSessionLocked(sessionID)
	}
	sess.MergeState(delta)
	return nil
}

// createSessionLocked allocates and stores a new session; caller must already
// hold the write lock. Internal helper used by Get/Create/Append paths.
func (s *InMemoryStore) createSessionLocked(sessionID string) *core.Session {
	sess := core.NewSession(sessionID)
	s.sessions[sessionID] = sess
	return sess
}
