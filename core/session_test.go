package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_NewDefaults(t *testing.T) {
	s := NewSession("app1", "user1", "sess1")
	require.NotNil(t, s)

	assert.Equal(t, "sess1", s.ID())
	assert.Empty(t, s.State())
	assert.Empty(t, s.Events())
}

func TestSession_SetGetState_UpdatesTimestamp(t *testing.T) {
	s := NewSession("app2", "user2", "sess2")
	prev := s.UpdatedAt()

	s.SetState("foo", "bar")
	v, ok := s.GetState("foo")
	require.True(t, ok)
	assert.Equal(t, any("bar"), v)
	assert.NotEqual(t, prev, s.UpdatedAt())
}

func TestSession_MergeState(t *testing.T) {
	s := NewSession("app3", "user3", "sess3")
	prev := s.UpdatedAt()

	s.MergeState(map[string]any{"a": 1, "b": 2})
	ss := s.State()
	assert.Equal(t, any(1), ss["a"])
	assert.Equal(t, any(2), ss["b"])
	assert.NotEqual(t, prev, s.UpdatedAt())
}

func TestSession_AddEvent_MergesState(t *testing.T) {
	s := NewSession("app4", "user4", "sess4")
	prev := s.UpdatedAt()

	ev := &Event{Actions: EventActions{StateDelta: Map(map[string]any{"x": 42})}}
	s.AddEvent(ev)

	// State merged
	v, ok := s.GetState("x")
	require.True(t, ok)
	assert.Equal(t, any(42), v)
	// Event appended
	assert.Len(t, s.Events(), 1)
	// Timestamp updated
	assert.NotEqual(t, prev, s.UpdatedAt())
}

func TestSession_AddEvents_MergesAndAppends(t *testing.T) {
	s := NewSession("app5", "user5", "sess5")
	e1 := &Event{Actions: EventActions{StateDelta: Map(map[string]any{"a": 1})}}
	e2 := &Event{Actions: EventActions{StateDelta: Map(map[string]any{"b": 2})}}

	s.AddEvents(e1, e2)

	ss := s.State()
	assert.Equal(t, any(1), ss["a"])
	assert.Equal(t, any(2), ss["b"])
	assert.Len(t, s.Events(), 2)
}

func TestSession_Clone_DeepCopy(t *testing.T) {
	s := NewSession("app7", "user7", "sess7")
	s.SetState("k", 1)
	s.AddEvent(&Event{Actions: EventActions{}})

	clone := s.Clone()
	require.NotNil(t, clone)
	// Changing clone's state doesn't affect original
	clone.SetState("k", 2)
	v, _ := s.GetState("k")
	assert.Equal(t, any(1), v)
	// Appending to clone's events doesn't affect original
	clone.AddEvent(&Event{Actions: EventActions{}})
	assert.Len(t, s.Events(), 1)
	assert.Len(t, clone.Events(), 2)
}

func TestSession_JSONRoundTrip(t *testing.T) {
	s := NewSession("app8", "user8", "sess8")
	s.SetState("a", 1)
	s.AddEvent(&Event{Actions: EventActions{StateDelta: Map(map[string]any{"b": 2})}})

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var out Session
	require.NoError(t, json.Unmarshal(data, &out))

	// ID and timestamps preserved
	assert.Equal(t, s.ID(), out.ID())
	assert.True(t, s.UpdatedAt().Equal(out.UpdatedAt()))

	// State and events preserved
	sv, _ := s.GetState("a")
	ov, _ := out.GetState("a")
	assert.EqualValues(t, sv, ov)
	assert.Len(t, out.Events(), len(s.Events()))
}

func TestSession_TimestampsMonotonicity(t *testing.T) {
	s := NewSession("app9", "user9", "sess9")
	t1 := s.UpdatedAt()
	time.Sleep(2 * time.Millisecond)
	s.SetState("n", 1)
	t2 := s.UpdatedAt()
	assert.True(t, t2.After(t1))
}

func TestSession_AddEvent_IgnoresPartial(t *testing.T) {
	s := NewSession("app-partial-1", "user-partial-1", "sess-partial-1")
	prev := s.UpdatedAt()

	ev := &Event{Partial: Bool(true), Actions: EventActions{StateDelta: Map(map[string]any{"x": 1})}}
	s.AddEvent(ev)

	// No changes expected
	assert.Equal(t, prev, s.UpdatedAt())
	assert.Empty(t, s.Events())
	_, ok := s.GetState("x")
	assert.False(t, ok)
}

func TestSession_AddEvents_IgnoresPartialAndMergesNonPartial(t *testing.T) {
	s := NewSession("app-partial-2", "user-partial-2", "sess-partial-2")
	prev := s.UpdatedAt()

	ePartial := &Event{Partial: Bool(true), Actions: EventActions{StateDelta: Map(map[string]any{"a": 1})}}
	eNon := &Event{Actions: EventActions{StateDelta: Map(map[string]any{"b": 2})}}

	s.AddEvents(ePartial, eNon)

	// Only non-partial appended and merged
	evs := s.Events()
	require.Len(t, evs, 1)

	ss := s.State()
	_, hasA := ss["a"]

	assert.False(t, hasA)
	assert.Equal(t, any(2), ss["b"])
	assert.NotEqual(t, prev, s.UpdatedAt())
}

func TestSession_AddEvents_AllPartial_NoChange(t *testing.T) {
	s := NewSession("app-partial-3", "user-partial-3", "sess-partial-3")

	prev := s.UpdatedAt()

	s.AddEvents(&Event{Partial: Bool(true)}, &Event{Partial: Bool(true)})

	assert.Equal(t, prev, s.UpdatedAt())
	assert.Empty(t, s.Events())
}
