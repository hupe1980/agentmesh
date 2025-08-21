package core

import "testing"

func TestRunContext_EmitEventStateAndArtifacts(t *testing.T) {
	ic, emitCh := newRunContextForTest()
	ic.SetState("foo", "bar")
	ic.AddArtifact("file1")

	ev := NewAssistantEvent(ic.RunID, "agent1", Content{}, false)

	if err := ic.EmitEvent(ev); err != nil {
		t.Fatalf("EmitEvent error: %v", err)
	}

	received := <-emitCh

	if received.Actions.StateDelta["foo"].(string) != "bar" {
		t.Fatalf("State delta missing: %+v", received.Actions)
	}

	if received.Actions.ArtifactDelta["file1"] != 1 {
		t.Fatalf("Artifact delta missing: %+v", received.Actions)
	}

	if len(ic.StateDelta) != 0 || len(ic.Artifacts) != 0 {
		t.Fatal("StateDelta & Artifacts should clear after emit")
	}
}

func TestRunContext_CloneIsolation(t *testing.T) {
	ic, _ := newRunContextForTest()
	ic.SetState("a", 1)
	ic.AddArtifact("f1")
	clone := ic.Clone()
	if clone.Session != ic.Session {
		t.Error("Session pointer should be shared")
	}
	clone.SetState("b", 2)
	if _, exists := ic.StateDelta["b"]; exists {
		t.Error("Original should not have clone's new state")
	}
	if v, _ := clone.GetState("a"); v.(int) != 1 {
		t.Error("Clone missing original state")
	}
}
