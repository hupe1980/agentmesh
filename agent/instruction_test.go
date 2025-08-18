package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
)

type mockProvider struct {
	text string
	err  error
}

func (m mockProvider) Instruction(*core.InvocationContext) (string, error) { return m.text, m.err }

func newTestInvocationContext() *core.InvocationContext {
	sess := core.NewSession("test-session")
	baseContent := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "hello"}}}
	return core.NewInvocationContext(
		context.Background(),
		sess.ID,
		"invocation-id",
		core.AgentInfo{Name: "TestAgent", Type: "test"},
		baseContent,
		make(chan core.Event, 1),
		nil,
		sess,
		nil,
		nil,
		nil,
		logging.NoOpLogger{},
	)
}

func TestInstruction_Static(t *testing.T) {
	inst := NewInstructionFromText("static instruction")
	if !inst.IsStatic() {
		t.Fatalf("expected static instruction")
	}
	got, err := inst.Resolve(newTestInvocationContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "static instruction" {
		t.Fatalf("expected 'static instruction', got %q", got)
	}
}

func TestInstruction_NewInstructionFromFunc(t *testing.T) {
	inst := NewInstructionFromFunc(func(_ *core.InvocationContext) (string, error) { return "dynamic via func", nil })
	if inst.IsStatic() {
		t.Fatalf("expected dynamic instruction")
	}
	got, err := inst.Resolve(newTestInvocationContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "dynamic via func" {
		t.Fatalf("expected 'dynamic via func', got %q", got)
	}
}

func TestInstruction_NewInstructionFromProvider(t *testing.T) {
	inst := NewInstructionFromProvider(mockProvider{text: "provider text"})
	if inst.IsStatic() {
		t.Fatalf("expected dynamic instruction")
	}
	got, err := inst.Resolve(newTestInvocationContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "provider text" {
		t.Fatalf("expected 'provider text', got %q", got)
	}
}

func TestInstruction_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("boom")
	inst := NewInstructionFromProvider(mockProvider{err: expectedErr})
	_, err := inst.Resolve(newTestInvocationContext())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
