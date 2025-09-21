package core

import (
	"context"
	"encoding/json"
	"maps"
	"time"
)

// Session represents a conversational container tracking mutable key/value
// state plus an ordered event history. Thread-safe and JSON serializable.
type Session struct {
	appName string
	userID  string
	id      string
	updated time.Time
	state   map[string]any
	events  []*Event
}

// NewSession creates a new session with the given ID.
func NewSession(appName, userID, id string) *Session {
	now := time.Now()

	return &Session{
		appName: appName,
		userID:  userID,
		id:      id,
		state:   map[string]any{},
		events:  []*Event{},
		updated: now,
	}
}

// AppName returns the application name.
func (s *Session) AppName() string { return s.appName }

// UserID returns the user ID.
func (s *Session) UserID() string { return s.userID }

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// UpdatedAt returns the last updated time.
func (s *Session) UpdatedAt() time.Time {
	return s.updated
}

// GetState returns the value and existence flag for a state key.
func (s *Session) GetState(key string) (any, bool) {
	v, ok := s.state[key]
	return v, ok
}

// SetState sets a key/value pair in session state updating the Updated timestamp.
func (s *Session) SetState(key string, value any) {
	s.state[key] = value
	s.updated = time.Now()
}

// MergeState merges the provided key/value pairs into the session state and updates the Updated timestamp.
func (s *Session) MergeState(delta map[string]any) {
	if delta == nil {
		return
	}

	maps.Copy(s.state, delta)
	s.updated = time.Now()
}

// AddEvents appends multiple events to the history, updating the Updated timestamp.
func (s *Session) AddEvents(events ...*Event) {
	// Preprocess outside lock
	filtered := make([]*Event, 0, len(events))
	merged := make(map[string]any)
	for _, ev := range events {
		// Ignore partial events
		if ev.IsPartial() {
			continue
		}

		if sd := ev.Actions.StateDelta.Or(nil); sd != nil {
			// last win semantics preserved by iteration order
			maps.Copy(merged, sd)
		}

		filtered = append(filtered, ev)
	}

	if len(filtered) == 0 {
		return
	}

	maps.Copy(s.state, merged)
	s.events = append(s.events, filtered...)
	s.updated = time.Now()
}

// AddEvent appends an event to the history updating Updated timestamp.
func (s *Session) AddEvent(ev *Event) {
	// Ignore partial events
	if ev.IsPartial() {
		return
	}

	if sd := ev.Actions.StateDelta.Or(nil); sd != nil {
		maps.Copy(s.state, sd)
	}

	s.events = append(s.events, ev)
	s.updated = ev.Timestamp
}

// Events returns the events in this session.
func (s *Session) Events() []*Event {
	return s.events
}

// Clone returns a deep copy of the session safe for independent mutation.
func (s *Session) Clone() *Session {
	clone := &Session{
		appName: s.appName,
		userID:  s.userID,
		id:      s.id,
		state:   make(map[string]any, len(s.state)),
		events:  make([]*Event, len(s.events)),
		updated: s.updated,
	}

	maps.Copy(clone.state, s.state)
	for i, ev := range s.events {
		if ev != nil {
			clone.events[i] = ev.Clone()
		}
	}

	return clone
}

// StateSnapshot returns a defensive copy of the current session state map.
// Modifications to the returned map do not affect the session.
func (s *Session) StateSnapshot() map[string]any {
	snapshot := make(map[string]any, len(s.state))
	maps.Copy(snapshot, s.state)

	return snapshot
}

// MarshalJSON implements custom JSON marshaling for private fields.
func (s *Session) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID      string         `json:"id"`
		Updated time.Time      `json:"updated"`
		State   map[string]any `json:"state"`
		Events  []*Event       `json:"events"`
	}{
		ID:      s.id,
		Updated: s.updated,
		State:   s.state,
		Events:  s.events,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for private fields.
func (s *Session) UnmarshalJSON(data []byte) error {
	aux := struct {
		ID      string         `json:"id"`
		Updated time.Time      `json:"updated"`
		State   map[string]any `json:"state"`
		Events  []*Event       `json:"events"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	s.id = aux.ID
	s.updated = aux.Updated
	s.state = aux.State
	s.events = aux.Events

	return nil
}

// SessionStore persists sessions and their evolving event history.
type SessionStore interface {
	// GetOrCreate retrieves an existing session or creates a new one.
	GetOrCreate(ctx context.Context, appName, userID, sessionID string) (*Session, error)

	// AppendEvent adds a new event to the session's event history.
	AppendEvent(ctx context.Context, session *Session, event *Event) error

	// Close releases any resources held by the store.
	Close() error
}
