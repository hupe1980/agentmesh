package graph_test

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestCommandBuilder_Goto(t *testing.T) {
	t.Run("WithUpdates", func(t *testing.T) {
		key := state.NewKey("test", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, "value")

		cmd, err := builder.Goto("next")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd == nil {
			t.Fatal("Expected command, got nil")
		}
		if len(cmd.Goto) != 1 || cmd.Goto[0] != "next" {
			t.Errorf("Expected Goto=['next'], got %v", cmd.Goto)
		}
		if cmd.Updates["test"] != "value" {
			t.Errorf("Expected updates['test']='value', got %v", cmd.Updates["test"])
		}
	})

	t.Run("NoUpdates", func(t *testing.T) {
		builder := graph.NewCommand()

		cmd, err := builder.Goto("next")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(cmd.Updates) != 0 {
			t.Errorf("Expected empty updates, got %v", cmd.Updates)
		}
	})
}

func TestCommandBuilder_GotoAll(t *testing.T) {
	key := state.NewKey("test", "")
	builder := graph.NewCommand()
	graph.CommandSet(builder, key, "value")

	cmd, err := builder.GotoAll("task1", "task2", "task3")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(cmd.Goto) != 3 {
		t.Errorf("Expected 3 targets, got %d", len(cmd.Goto))
	}
}

func TestCommandBuilder_End(t *testing.T) {
	key := state.NewKey("result", "")
	builder := graph.NewCommand()
	graph.CommandSet(builder, key, "done")

	cmd, err := builder.End()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(cmd.Goto) != 1 || cmd.Goto[0] != graph.EndNode {
		t.Errorf("Expected Goto=[EndNode], got %v", cmd.Goto)
	}
}

func TestCommandBuilder_DuplicateKey(t *testing.T) {
	key := state.NewKey("test", "")
	builder := graph.NewCommand()
	graph.CommandSet(builder, key, "value1")
	graph.CommandSet(builder, key, "value2") // Duplicate

	_, err := builder.Goto("next")
	if err == nil {
		t.Fatal("Expected error for duplicate key, got nil")
	}
}

func TestCommandBuilder_TypedOperations(t *testing.T) {
	counterKey := state.NewKey("counter", 0)
	messagesKey := state.NewListKey[string]("messages", 10)

	builder := graph.NewCommand()
	graph.CommandSet(builder, counterKey, 42)
	graph.CommandAppend(builder, messagesKey, "msg1", "msg2")

	cmd, err := builder.End()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if cmd.Updates["counter"] != 42 {
		t.Errorf("Expected counter=42, got %v", cmd.Updates["counter"])
	}
}
