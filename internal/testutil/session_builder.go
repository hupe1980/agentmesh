package testutil

import (
	"github.com/hupe1980/agentmesh/core"
)

// SessionBuilder helps construct sessions with fluent chaining for tests.
// Example:
//
//	sess := NewSessionBuilder("sess-1").State("k","v").Events(ev1, ev2).Build()
type SessionBuilder struct {
	id     string
	state  map[string]any
	events []core.Event
}

// NewSessionBuilder creates a new builder for a session with the given id.
// Use chainable methods (State, Event, Events) then call Build.
func NewSessionBuilder(id string) *SessionBuilder {
	return &SessionBuilder{id: id, state: map[string]any{}}
}

// State sets or overwrites a state key/value pair on the resulting session (chainable).
func (b *SessionBuilder) State(key string, val any) *SessionBuilder {
	b.state[key] = val
	return b
}

// Event appends a single event to the session history (chainable).
func (b *SessionBuilder) Event(ev core.Event) *SessionBuilder {
	b.events = append(b.events, ev)
	return b
}

// Events appends multiple events to the session history (chainable).
func (b *SessionBuilder) Events(evs ...core.Event) *SessionBuilder {
	b.events = append(b.events, evs...)
	return b
}

// Build returns a *core.Session with pre-populated state and events.
func (b *SessionBuilder) Build() *core.Session {
	s := core.NewSession(b.id)

	for k, v := range b.state {
		s.State[k] = v
	}

	s.Events = append(s.Events, b.events...)

	return s
}
