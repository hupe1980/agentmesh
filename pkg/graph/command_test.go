package graph_test

import (
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ====================
// To Tests
// ====================

func TestTo(t *testing.T) {
	cmd, err := graph.To(graph.END)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Next) != 1 || cmd.Next[0] != graph.END {
		t.Errorf("expected [END], got %v", cmd.Next)
	}
	if cmd.Updates != nil {
		t.Errorf("expected nil updates, got %v", cmd.Updates)
	}
}

func TestToMultiple(t *testing.T) {
	cmd, err := graph.To("a", "b", "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Next) != 3 {
		t.Errorf("expected 3 targets, got %d", len(cmd.Next))
	}
	if cmd.Next[0] != "a" || cmd.Next[1] != "b" || cmd.Next[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", cmd.Next)
	}
}

func TestToEmpty(t *testing.T) {
	cmd, err := graph.To()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Next) != 0 {
		t.Errorf("expected empty targets, got %v", cmd.Next)
	}
}

// ====================
// Fail Tests
// ====================

func TestFail(t *testing.T) {
	expectedErr := errors.New("test error")
	cmd, err := graph.Fail(expectedErr)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if cmd != nil {
		t.Errorf("expected nil command, got %+v", cmd)
	}
}

func TestFailNil(t *testing.T) {
	cmd, err := graph.Fail(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if cmd != nil {
		t.Errorf("expected nil command, got %+v", cmd)
	}
}

// ====================
// Set Tests
// ====================

func TestSet(t *testing.T) {
	key := graph.NewKey[int]("counter")
	cmd, err := graph.Set(key, 42).To("next")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Updates["counter"] != 42 {
		t.Errorf("expected counter=42, got %v", cmd.Updates["counter"])
	}
	if len(cmd.Next) != 1 || cmd.Next[0] != "next" {
		t.Errorf("expected [next], got %v", cmd.Next)
	}
}

func TestSetChain(t *testing.T) {
	key1 := graph.NewKey[string]("status")
	key2 := graph.NewKey[int]("count")

	cmd, err := graph.Set(key1, "done").
		With(graph.SetValue(key2, 42)).
		To("next")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Updates["status"] != "done" {
		t.Errorf("expected status=done, got %v", cmd.Updates["status"])
	}
	if cmd.Updates["count"] != 42 {
		t.Errorf("expected count=42, got %v", cmd.Updates["count"])
	}
}

// ====================
// List Key Set Tests
// ====================

func TestSetListKey(t *testing.T) {
	key := graph.NewListKey[string]("test_messages")
	cmd, err := graph.Set(key, []string{"hello", "world"}).To("next")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items, ok := cmd.Updates["test_messages"].([]string)
	if !ok {
		t.Fatalf("expected []string type, got %T", cmd.Updates["test_messages"])
	}
	if len(items) != 2 || items[0] != "hello" || items[1] != "world" {
		t.Errorf("expected [hello, world], got %v", items)
	}
}

// ====================
// Cmd Builder Tests
// ====================

func TestCmdBuilder(t *testing.T) {
	key := graph.NewKey[string]("status")
	cmd, err := graph.Cmd().
		With(graph.SetValue(key, "done")).
		To("next")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Updates["status"] != "done" {
		t.Errorf("expected status=done, got %v", cmd.Updates["status"])
	}
}

func TestCmdBuilderMultipleWith(t *testing.T) {
	statusKey := graph.NewKey[string]("status")
	countKey := graph.NewKey[int]("count")
	enabledKey := graph.NewKey[bool]("enabled")

	cmd, err := graph.Cmd().
		With(graph.SetValue(statusKey, "done")).
		With(graph.SetValue(countKey, 42)).
		With(graph.SetValue(enabledKey, true)).
		To("next")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Updates["status"] != "done" {
		t.Errorf("expected status=done, got %v", cmd.Updates["status"])
	}
	if cmd.Updates["count"] != 42 {
		t.Errorf("expected count=42, got %v", cmd.Updates["count"])
	}
	if cmd.Updates["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", cmd.Updates["enabled"])
	}
}

func TestCmdEnd(t *testing.T) {
	cmd, err := graph.Cmd().End()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Next) != 1 || cmd.Next[0] != graph.END {
		t.Errorf("expected [__end__], got %v", cmd.Next)
	}
}

func TestCmdSetListValue(t *testing.T) {
	messagesKey := graph.NewListKey[string]("test_messages")

	cmd, err := graph.Cmd().
		With(graph.SetValue(messagesKey, []string{"hello", "world"})).
		To("next")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs, ok := cmd.Updates["test_messages"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", cmd.Updates["test_messages"])
	}
	if len(msgs) != 2 || msgs[0] != "hello" || msgs[1] != "world" {
		t.Errorf("expected [hello, world], got %v", msgs)
	}
}

// ====================
// Conditional Pattern Tests
// ====================

func TestConditionalPattern(t *testing.T) {
	statusKey := graph.NewKey[string]("status")

	// Pattern: build command incrementally, then decide routing
	processWithMaxTurns := func(turn, maxTurns int) (*graph.Command, error) {
		cmd := graph.Cmd()
		if turn >= maxTurns {
			cmd.With(graph.SetValue(statusKey, "max_reached"))
			return cmd.End()
		}
		cmd.With(graph.SetValue(statusKey, "continue"))
		return cmd.To("next")
	}

	// Test max turns reached
	cmd, err := processWithMaxTurns(5, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Updates["status"] != "max_reached" {
		t.Errorf("expected status=max_reached, got %v", cmd.Updates["status"])
	}
	if cmd.Next[0] != graph.END {
		t.Errorf("expected END, got %v", cmd.Next[0])
	}

	// Test continue
	cmd, err = processWithMaxTurns(3, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Updates["status"] != "continue" {
		t.Errorf("expected status=continue, got %v", cmd.Updates["status"])
	}
	if cmd.Next[0] != "next" {
		t.Errorf("expected next, got %v", cmd.Next[0])
	}
}

// ====================
// Parallel Branching Tests
// ====================

func TestParallelBranching(t *testing.T) {
	cmd, err := graph.Cmd().To("branch_a", "branch_b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Next) != 2 {
		t.Errorf("expected 2 targets, got %d", len(cmd.Next))
	}
}

func TestEmptyUpdates(t *testing.T) {
	cmd, err := graph.Cmd().To("next")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Updates) != 0 {
		t.Errorf("expected empty updates, got %v", cmd.Updates)
	}
}
