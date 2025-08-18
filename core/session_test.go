package core

import "testing"

func TestSession_ApplyStateDeltaAndClone(t *testing.T) {
	s := NewSession("s1")

	delta := map[string]any{"a": 1, "b": "x"}

	s.ApplyStateDelta(delta)
	if v, ok := s.GetState("a"); !ok || v.(int) != 1 {
		t.Fatalf("State not applied: %+v", s.State)
	}

	clone := s.Clone()
	if clone == s {
		t.Error("Clone should be a different pointer")
	}

	clone.SetState("c", 2)
	if _, exists := s.GetState("c"); exists {
		t.Error("Original should not have clone's new key")
	}
}

func TestSession_AddEventAndHistory(t *testing.T) {
	userEv := NewUserMessageEvent("inv-123", "hi")
	assistantEv := NewMessageEvent("assistant", "hello")
	assistantEv.Author = "assistant"
	s := NewSession("s2")
	s.AddEvent(assistantEv)
	s.AddEvent(userEv)
	all := s.GetEvents()
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}
	orig := all[0].Author
	all[0].Author = "changed"
	if s.GetEvents()[0].Author != orig {
		t.Error("events slice should be copied on read")
	}
	history := s.GetConversationHistory()
	foundUser := false
	for _, hev := range history {
		if hev.Content != nil && hev.Content.Role == "user" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Error("expected user event in history")
	}
}
