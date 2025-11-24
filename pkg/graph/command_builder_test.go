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

func TestCommandBuilder_GotoIf(t *testing.T) {
	t.Run("TrueConditionWithUpdates", func(t *testing.T) {
		key := state.NewKey("test", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, "value")

		cmd, err := builder.GotoIf("next", true)
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

	t.Run("FalseCondition", func(t *testing.T) {
		key := state.NewKey("test", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, "value")

		cmd, err := builder.GotoIf("next", false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd != nil {
			t.Errorf("Expected nil command for false condition, got %v", cmd)
		}
	})

	t.Run("ChainedConditionals", func(t *testing.T) {
		// Simulate tool call routing logic
		hasToolCalls := true
		key := state.NewKey("status", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, "processing")

		// First condition matches
		cmd, err := builder.GotoIf("tool", hasToolCalls)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd == nil {
			t.Fatal("Expected command for true condition")
		}
		if cmd.Goto[0] != "tool" {
			t.Errorf("Expected route to 'tool', got %v", cmd.Goto[0])
		}
	})

	t.Run("FallbackRouting", func(t *testing.T) {
		// Simulate fallback pattern
		hasToolCalls := false
		key := state.NewKey("status", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, "done")

		// First condition doesn't match
		cmd, err := builder.GotoIf("tool", hasToolCalls)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd != nil {
			t.Fatal("Expected nil for false condition")
		}

		// Fallback condition
		cmd, err = builder.GotoIf("end", !hasToolCalls)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd == nil {
			t.Fatal("Expected command for fallback")
		}
		if cmd.Goto[0] != "end" {
			t.Errorf("Expected route to 'end', got %v", cmd.Goto[0])
		}
	})

	t.Run("ErrorPropagation", func(t *testing.T) {
		key := state.NewKey("test", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, "value1")
		graph.CommandSet(builder, key, "value2") // Duplicate key error

		_, err := builder.GotoIf("next", true)
		if err == nil {
			t.Fatal("Expected error for duplicate key, got nil")
		}
	})

	t.Run("MultiWayRouting", func(t *testing.T) {
		// Simulate priority-based routing
		priority := "medium"
		key := state.NewKey("priority", "")
		builder := graph.NewCommand()
		graph.CommandSet(builder, key, priority)

		// Check high priority
		cmd, err := builder.GotoIf("urgent", priority == "high")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd != nil {
			t.Fatal("Expected nil for high priority check")
		}

		// Check medium priority - should match
		cmd, err = builder.GotoIf("normal", priority == "medium")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cmd == nil {
			t.Fatal("Expected command for medium priority")
		}
		if cmd.Goto[0] != "normal" {
			t.Errorf("Expected route to 'normal', got %v", cmd.Goto[0])
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
