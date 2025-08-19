package core

import (
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Event constructor & helper method tests
func TestEvent_ConstructorsAndMethods(t *testing.T) {
	e := NewEvent("inv-123", "authorA")
	require.NotEmpty(t, e.ID, "event ID should be set")
	require.False(t, e.Timestamp.IsZero(), "timestamp should be initialized")
	assert.Equal(t, "authorA", e.Author)
	assert.Equal(t, "inv-123", e.InvocationID)

	msg := NewMessageEvent("agent1", "hello world")
	require.NotNil(t, msg.Content)
	assert.Equal(t, "assistant", msg.Content.Role)
	assert.Len(t, msg.Content.Parts, 1)

	user := NewUserMessageEvent("inv-123", "hi")
	require.NotNil(t, user.Content)
	assert.Equal(t, "user", user.Content.Role)
	assert.Equal(t, "inv-123", user.InvocationID)

	callArgs := "test"
	fCall := NewFunctionCallEvent("agent2", "do_stuff", callArgs)
	calls := fCall.GetFunctionCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "do_stuff", calls[0].Name)
	assert.Equal(t, callArgs, calls[0].Arguments)

	fRespOK := NewFunctionResponseEvent("agent2", "call-1", "do_stuff", 42, nil)
	resps := fRespOK.GetFunctionResponses()
	require.Len(t, resps, 1)
	require.IsType(t, 42, resps[0].Response)
	assert.Equal(t, 42, resps[0].Response.(int))
	assert.Empty(t, resps[0].Error)

	fRespErr := NewFunctionResponseEvent("agent2", "call-2", "do_stuff", nil, errors.New("boom"))
	resps = fRespErr.GetFunctionResponses()
	require.Len(t, resps, 1)
	assert.NotEmpty(t, resps[0].Error)
}

func TestEvent_IsFinalResponseLogic(t *testing.T) {
	e := NewEvent("inv", "authorA")
	assert.True(t, e.IsFinalResponse(), "basic event should be final")

	partial := true
	e2 := NewEvent("inv", "agent")
	e2.Partial = &partial
	assert.False(t, e2.IsFinalResponse(), "partial event should not be final")

	e3 := NewFunctionCallEvent("agent", "f", "")
	assert.False(t, e3.IsFinalResponse(), "event with function call should not be final")

	e4 := NewFunctionResponseEvent("agent", "call-3", "f", "ok", nil)
	assert.False(t, e4.IsFinalResponse(), "event with function response should not be final")

	skip := true
	e5 := NewEvent("inv", "agent")
	e5.Partial = &partial
	e5.Actions.SkipSummarization = &skip
	assert.True(t, e5.IsFinalResponse(), "SkipSummarization should force final")

	e6 := NewEvent("inv", "agent")
	e6.LongRunningToolIDs = []string{"tool1"}
	assert.True(t, e6.IsFinalResponse(), "long running tool should mark final")
}

func TestEvent_IDUniqueness(t *testing.T) {
	a := util.NewID()
	b := util.NewID()
	assert.NotEqual(t, a, b, "expected unique IDs")
}

// IO Parts discrimination tests
func TestParts_DiscriminatedUnion(t *testing.T) {
	parts := []Part{
		TextPart{Text: "hello"},
		DataPart{Data: map[string]any{"k": "v"}},
		FilePart{File: FilePartFile{URI: "file://x"}},
		FunctionCallPart{FunctionCall: FunctionCall{Name: "f"}},
		FunctionResponsePart{FunctionResponse: FunctionResponse{Name: "f"}},
	}
	for _, p := range parts {
		switch pt := p.(type) {
		case TextPart, DataPart, FilePart, FunctionCallPart, FunctionResponsePart:
			// expected
		default:
			require.Failf(t, "unexpected part type", "%T (%v)", pt, pt)
		}
	}
}
