package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Event constructor & helper method tests
func TestEvent_ConstructorsAndMethods(t *testing.T) {
	e := NewFullAssistantEvent("inv-123", "authorA")
	require.NotEmpty(t, e.ID, "event ID should be set")
	require.False(t, e.Timestamp.IsZero(), "timestamp should be initialized")
	assert.Equal(t, "authorA", e.Author)
	assert.Equal(t, "inv-123", e.RunID)

	fRespOK := NewFunctionResponseEvent("inv-123", "agent2", "call-1", "do_stuff", 42)
	resps := fRespOK.GetFunctionResponses()
	require.Len(t, resps, 1)
	require.IsType(t, 42, resps[0].Response)
	assert.Equal(t, 42, resps[0].Response.(int))

	fRespErr := NewFunctionResponseEvent("inv-123", "agent2", "call-2", "do_stuff", nil)
	resps = fRespErr.GetFunctionResponses()
	require.Len(t, resps, 1)
}

func TestEvent_IsFinalResponseLogic(t *testing.T) {
	e := NewFullAssistantEvent("inv-123", "authorA")
	assert.True(t, e.IsFinalResponse(), "basic event should be final")

	e2 := NewPartialAssistantEvent("inv-123", "authorA")
	assert.False(t, e2.IsFinalResponse(), "partial event should not be final")

	e3 := NewFunctionResponseEvent("inv-123", "agent", "call-3", "f", "ok")
	assert.False(t, e3.IsFinalResponse(), "event with function response should not be final")

	e4 := NewFunctionResponseEvent("inv-123", "agent", "call-3", "f", "ok")
	e4.Actions.SkipSummarization = Bool(true)
	assert.True(t, e4.IsFinalResponse(), "SkipSummarization should force final")
}

// TestEventClone_DeepCopies verifies Event.Clone creates independent copies of maps,
// option pointers, and content/parts.
func TestEventClone_DeepCopies(t *testing.T) {
	// Build rich content with multiple part types
	text := &TextPart{Text: "hello"}
	data := &DataPart{Data: map[string]any{"d1": 1}}
	file := &FilePart{File: &FileRawBytes{Bytes: []byte("abc")}}
	fcall := &FunctionCallPart{
		FunctionCall: &FunctionCall{
			ID:        "c1",
			Name:      "tool",
			Arguments: "{}",
		},
	}
	fresp := &FunctionResponsePart{
		FunctionResponse: &FunctionResponse{
			ID:       "c1",
			Name:     "tool",
			Response: 42,
		},
	}

	// Construct event with maps and option pointers populated
	e := NewFullAssistantEvent("run-1", "agent-a", text, data, file, fcall, fresp)
	e.Branch = String("root.child")
	e.Partial = Bool(false)
	e.Actions.SkipSummarization = Bool(true)
	e.Actions.TransferToAgent = String("other")
	e.Actions.Escalate = Bool(true)
	e.Actions.StateDelta = Map(map[string]any{"a": 1, "b": 2})
	e.Actions.ArtifactDelta = Map(map[string]int{"x": 10})
	e.CustomMetadata = Map(map[string]string{"k": "v"})

	// Clone
	c := e.Clone()
	require.NotNil(t, c)

	// Mutate original deeply
	sd := e.Actions.StateDelta.Or(nil)
	sd["a"] = 99
	e.Actions.StateDelta = Map(sd)
	ad := e.Actions.ArtifactDelta.Or(nil)
	ad["x"] = 77
	e.Actions.ArtifactDelta = Map(ad)
	e.Actions.SkipSummarization.Set(false)
	e.Actions.Escalate = Bool(false)
	e.Actions.TransferToAgent = String("changed")
	cm := e.CustomMetadata.Or(nil)
	cm["k"] = "changed"
	e.CustomMetadata = Map(cm)

	// mutate parts
	text.Text = "changed"
	data.Data["d1"] = 999
	// mutate underlying file bytes and function fields
	if rb, ok := file.File.(*FileRawBytes); ok {
		rb.Bytes[0] = 'Z'
	}
	fcall.FunctionCall.Name = "changed"
	fresp.FunctionResponse.Response = 99
	e.Parts = append(e.Parts, &TextPart{Text: "new"})

	// Assertions: clone remains with original values
	// Maps should be unaffected
	assert.Equal(t, any(1), c.Actions.StateDelta.Or(nil)["a"])
	assert.Equal(t, 10, c.Actions.ArtifactDelta.Or(nil)["x"])
	assert.Equal(t, "v", c.CustomMetadata.Or(nil)["k"])

	// Option pointers independent
	require.NotNil(t, c.Actions.SkipSummarization)
	assert.True(t, c.Actions.SkipSummarization.Or(false))
	require.NotNil(t, c.Actions.Escalate)
	assert.True(t, c.Actions.Escalate.Or(false))
	require.NotNil(t, c.Actions.TransferToAgent)
	assert.Equal(t, "other", c.Actions.TransferToAgent.Or(""))

	// Content parts are deep-cloned; verify unchanged and length unaffected
	// original had 5 parts; append should not affect clone
	require.Len(t, c.Parts, 5)

	// TextPart
	ct0, ok := c.Parts[0].(*TextPart)
	require.True(t, ok)
	assert.Equal(t, "hello", ct0.Text)

	// DataPart
	cd1, ok := c.Parts[1].(*DataPart)
	require.True(t, ok)
	assert.Equal(t, any(1), cd1.Data["d1"])

	// FilePart clone still has original bytes
	cf2, ok := c.Parts[2].(*FilePart)
	require.True(t, ok)
	if rb, ok2 := cf2.File.(*FileRawBytes); ok2 {
		assert.Equal(t, []byte("abc"), rb.Bytes)
	}

	// FunctionCallPart is deep-cloned; original mutation shouldn't affect clone
	cc3, ok := c.Parts[3].(*FunctionCallPart)
	require.True(t, ok)
	assert.Equal(t, "tool", cc3.FunctionCall.Name)

	// FunctionResponsePart is deep-cloned; original mutation shouldn't affect clone
	cr4, ok := c.Parts[4].(*FunctionResponsePart)
	require.True(t, ok)
	assert.Equal(t, 42, cr4.FunctionResponse.Response)
}

// TestEventClone_NilSafety ensures Clone works with nil content and nil maps.
func TestEventClone_NilSafety(t *testing.T) {
	e := &Event{ID: "e1", Actions: EventActions{}}
	c := e.Clone()
	require.NotNil(t, c)
	assert.Nil(t, c.Actions.StateDelta.Or(nil))
	assert.Nil(t, c.Actions.ArtifactDelta.Or(nil))
	assert.Nil(t, c.CustomMetadata.Or(nil))
}

func TestEvent_ApplyActions_MergeAndSet(t *testing.T) {
	// Given an empty event and a set of actions
	ev := &Event{Actions: EventActions{}}

	transfer := "OtherAgent"
	esc := true

	actions := EventActions{
		StateDelta:        Map(map[string]any{"a": 1, "b": 2}),
		ArtifactDelta:     Map(map[string]int{"x": 10}),
		TransferToAgent:   String(transfer),
		Escalate:          Bool(esc),
		SkipSummarization: Bool(true),
	}

	// When
	ev.ApplyActions(&actions)

	// Then: state delta copied
	assert.Equal(t, any(1), ev.Actions.StateDelta.Or(nil)["a"])
	assert.Equal(t, any(2), ev.Actions.StateDelta.Or(nil)["b"])

	// Then: artifact delta copied
	assert.Equal(t, 10, ev.Actions.ArtifactDelta.Or(nil)["x"])

	// Then: transfer/escalate/skip set
	assert.NotNil(t, ev.Actions.TransferToAgent)
	assert.Equal(t, transfer, ev.Actions.TransferToAgent.Or(""))
	assert.NotNil(t, ev.Actions.Escalate)
	assert.True(t, ev.Actions.Escalate.Or(false))
	assert.NotNil(t, ev.Actions.SkipSummarization)
	assert.True(t, ev.Actions.SkipSummarization.Or(false))
}

func TestEvent_ApplyActions_MergeWithExisting(t *testing.T) {
	// Given an event with existing deltas
	// Wrap initial maps in Opt using Map helper
	ev := &Event{
		Actions: EventActions{
			StateDelta:    Map(map[string]any{"a": 0}),
			ArtifactDelta: Map(map[string]int{"x": 1}),
		},
	}

	actions := EventActions{
		StateDelta:    Map(map[string]any{"a": 2, "b": 3}),
		ArtifactDelta: Map(map[string]int{"x": 4, "y": 5}),
	}

	// When
	ev.ApplyActions(&actions)

	// Then: merged with override
	state := ev.Actions.StateDelta.Or(nil)
	artifact := ev.Actions.ArtifactDelta.Or(nil)

	assert.Equal(t, any(2), state["a"])
	assert.Equal(t, any(3), state["b"])
	assert.Equal(t, 4, artifact["x"])
	assert.Equal(t, 5, artifact["y"])
}
