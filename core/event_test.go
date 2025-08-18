package core

import (
	"errors"
	"testing"
)

// Event constructor & helper method tests
func TestEvent_ConstructorsAndMethods(t *testing.T) {
	e := NewEvent("inv-123", "authorA")
	if e.Author != "authorA" || e.InvocationID != "inv-123" || e.ID == "" || e.Timestamp.IsZero() {
		t.Fatalf("NewEvent did not initialize fields correctly: %+v", e)
	}

	msg := NewMessageEvent("agent1", "hello world")
	if msg.Content == nil || msg.Content.Role != "assistant" || len(msg.Content.Parts) != 1 {
		t.Fatalf("NewMessageEvent malformed: %+v", msg)
	}

	user := NewUserMessageEvent("hi")
	if user.Content == nil || user.Content.Role != "user" {
		t.Fatalf("NewUserMessageEvent malformed: %+v", user)
	}

	callArgs := "test"
	fCall := NewFunctionCallEvent("agent2", "do_stuff", callArgs)
	calls := fCall.GetFunctionCalls()
	if len(calls) != 1 || calls[0].Name != "do_stuff" || calls[0].Arguments != callArgs {
		t.Fatalf("GetFunctionCalls extraction failed: %+v", calls)
	}

	fRespOK := NewFunctionResponseEvent("agent2", "call-1", "do_stuff", 42, nil)
	resps := fRespOK.GetFunctionResponses()
	if len(resps) != 1 || resps[0].Response.(int) != 42 || resps[0].Error != "" {
		t.Fatalf("Function response success extraction failed: %+v", resps)
	}

	fRespErr := NewFunctionResponseEvent("agent2", "call-2", "do_stuff", nil, errors.New("boom"))
	resps = fRespErr.GetFunctionResponses()
	if resps[0].Error == "" {
		t.Fatalf("Expected error message in function response: %+v", resps[0])
	}
}

func TestEvent_IsFinalResponseLogic(t *testing.T) {
	e := NewEvent("inv", "authorA")
	if !e.IsFinalResponse() {
		t.Error("Expected basic event to be final")
	}

	partial := true
	e2 := NewEvent("inv", "agent")
	e2.Partial = &partial
	if e2.IsFinalResponse() {
		t.Error("Partial event should not be final")
	}

	e3 := NewFunctionCallEvent("agent", "f", "")
	if e3.IsFinalResponse() {
		t.Error("Event with function call should not be final")
	}

	e4 := NewFunctionResponseEvent("agent", "call-3", "f", "ok", nil)
	if e4.IsFinalResponse() {
		t.Error("Event with function response should not be final")
	}

	skip := true
	e5 := NewEvent("inv", "agent")
	e5.Partial = &partial
	e5.Actions.SkipSummarization = &skip
	if !e5.IsFinalResponse() {
		t.Error("SkipSummarization should force final")
	}

	e6 := NewEvent("inv", "agent")
	e6.LongRunningToolIDs = []string{"tool1"}
	if !e6.IsFinalResponse() {
		t.Error("Long running tool should mark final")
	}
}

func TestEvent_IDUniqueness(t *testing.T) {
	a := NewID()
	b := NewID()
	if a == b {
		t.Error("Expected unique IDs")
	}
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
		default:
			t.Fatalf("Unexpected part type: %T (%v)", pt, pt)
		}
	}
}
