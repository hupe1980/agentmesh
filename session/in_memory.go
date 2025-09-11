package session

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertions)
var _ core.SessionStore = (*InMemoryStore)(nil)

// InMemoryStore is a volatile SessionStore implementation storing
// sessions in a process local nested map. It is safe for concurrent access and best
// suited for tests or ephemeral demo servers. Each returned session is cloned
// to prevent external mutation of internal state.
type InMemoryStore struct {
	sessions map[string]map[string]map[string]*core.Session // appName -> userID -> sessionID
	mu       sync.RWMutex
}

// NewInMemoryStore constructs an empty in‑memory session store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions: make(map[string]map[string]map[string]*core.Session),
	}
}

// GetOrCreate returns an existing session (clone) or creates a new one lazily.
func (s *InMemoryStore) GetOrCreate(_ context.Context, appName, userID, sessionID string) (*core.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure nested maps exist
	if _, ok := s.sessions[appName]; !ok {
		s.sessions[appName] = make(map[string]map[string]*core.Session)
	}

	if _, ok := s.sessions[appName][userID]; !ok {
		s.sessions[appName][userID] = make(map[string]*core.Session)
	}

	session, ok := s.sessions[appName][userID][sessionID]
	if !ok {
		session = core.NewSession(appName, userID, sessionID)
		s.sessions[appName][userID][sessionID] = session
	}

	return session.Clone(), nil
}

// AppendEvent adds an event to an existing session.
func (s *InMemoryStore) AppendEvent(_ context.Context, sess *core.Session, ev *core.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userSessions, ok := s.sessions[sess.AppName()]
	if !ok {
		return fmt.Errorf("%w: app_name=%s", core.ErrSessionNotFound, sess.AppName())
	}

	sessionMap, ok := userSessions[sess.UserID()]
	if !ok {
		return fmt.Errorf("%w: user_id=%s", core.ErrSessionNotFound, sess.UserID())
	}

	storedSess, ok := sessionMap[sess.ID()]
	if !ok {
		return fmt.Errorf("%w: session_id=%s", core.ErrSessionNotFound, sess.ID())
	}

	// Only after we've confirmed the session exists in the store, mutate both
	// the provided session and the stored session to reflect the append.
	// This avoids mutating the caller-owned session when returning an error.
	sess.AddEvent(ev)
	storedSess.AddEvent(ev)

	return nil
}

// Close implements core.SessionStore. No resources to release for in-memory store.
func (s *InMemoryStore) Close() error { return nil }
