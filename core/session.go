package core

import (
	"sync"
	"time"
)

// Session represents a conversational container tracking mutable key/value
// state plus an ordered event history. It is safe for concurrent access.
//
// Contract:
//   - State mutations update Updated timestamp
//   - GetEvents returns a defensive copy to avoid external mutation
//   - GetConversationHistory filters events to user/assistant/tool roles and
//     excludes partial streaming fragments
//   - Clone performs deep copies of maps/slices for safe divergence.
type Session struct {
	ID       string                 `json:"id"`
	State    map[string]interface{} `json:"state"`
	Events   []Event                `json:"events"`
	Created  time.Time              `json:"created"`
	Updated  time.Time              `json:"updated"`
	Metadata map[string]string      `json:"metadata"`
	mu       sync.RWMutex
}

// NewSession creates a new session with the given ID.
func NewSession(id string) *Session {
	now := time.Now()
	return &Session{ID: id, State: map[string]interface{}{}, Events: []Event{}, Created: now, Updated: now, Metadata: map[string]string{}}
}

func (s *Session) GetState(key string) (interface{}, bool) {
	// GetState returns the value and existence flag for a state key.
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.State[key]
	return v, ok
}

// SetState sets a key/value pair in session state updating the Updated timestamp.
func (s *Session) SetState(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State[key] = value
	s.Updated = time.Now()
}

// ApplyStateDelta merges the provided key/value pairs into State.
func (s *Session) ApplyStateDelta(delta map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range delta {
		s.State[k] = v
	}
	s.Updated = time.Now()
}

// AddEvent appends an event to the history updating Updated timestamp.
func (s *Session) AddEvent(ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, ev)
	s.Updated = time.Now()
}

// GetEvents returns a copy of the full event slice to prevent callers from
// mutating internal state.
// GetEvents returns a defensive copy of the full event slice.
func (s *Session) GetEvents() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]Event, len(s.Events))
	copy(events, s.Events)
	return events
}

// GetConversationHistory returns filtered events suitable for providing
// conversational context to models (excludes partials and non-conversational roles).
func (s *Session) GetConversationHistory() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	allowed := map[string]bool{"user": true, "assistant": true, "tool": true}
	res := make([]Event, 0, len(s.Events))
	for _, ev := range s.Events {
		if ev.Content == nil || !allowed[ev.Content.Role] {
			continue
		}
		if ev.Partial != nil && *ev.Partial {
			continue
		}
		res = append(res, ev)
	}
	return res
}

// Clone creates a deep copy of the session (maps & slices) except mutex.
// Clone returns a deep copy of the session safe for independent mutation.
func (s *Session) Clone() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	clone := &Session{ID: s.ID, State: make(map[string]interface{}, len(s.State)), Events: make([]Event, len(s.Events)), Created: s.Created, Updated: s.Updated, Metadata: make(map[string]string, len(s.Metadata))}
	for k, v := range s.State {
		clone.State[k] = v
	}
	copy(clone.Events, s.Events)
	for k, v := range s.Metadata {
		clone.Metadata[k] = v
	}
	return clone
}

// SessionStore persists sessions and their evolving state / event history.
type SessionStore interface {
	Create(id string) (*Session, error)
	Get(id string) (*Session, error)
	AppendEvent(sessionID string, event Event) error
	ApplyDelta(sessionID string, delta map[string]interface{}) error
}
