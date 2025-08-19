package core

import "testing"

func TestRunContext_EmitEventStateAndArtifacts(t *testing.T) {
	ic, emitCh := newRunContextForTest()
	ic.SetState("foo", "bar")
	ic.AddArtifact("file1")
	ev := NewEvent("agent1", ic.RunID)
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

func TestRunContext_CommitStateDelta(t *testing.T) {
	ic, _ := newRunContextForTest()
	sSvc := ic.SessionStore.(*icMockSessionService)
	ic.SetState("k1", 123)
	if err := ic.CommitStateDelta(); err != nil {
		t.Fatalf("CommitStateDelta error: %v", err)
	}
	if sSvc.applied == nil || sSvc.applied[ic.SessionID]["k1"].(int) != 123 {
		t.Fatalf("State delta not applied: %+v", sSvc.applied)
	}
	if len(ic.StateDelta) != 0 {
		t.Error("StateDelta should be cleared after commit")
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

func TestRunContext_WithBranch(t *testing.T) {
	ic, _ := newRunContextForTest()
	branched := ic.WithBranch("Root.Child")
	if branched.Branch != "Root.Child" {
		t.Errorf("Expected branch Root.Child, got %s", branched.Branch)
	}
	if ic.Branch != "" {
		t.Error("Original branch should remain empty")
	}
}
