package graph_test

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestUpdateBuilder_Build(t *testing.T) {
	t.Run("WithUpdates", func(t *testing.T) {
		key := state.NewKey("test", "")
		builder := graph.NewUpdate()
		graph.UpdateSet(builder, key, "value")

		updates, err := builder.Build()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if updates["test"] != "value" {
			t.Errorf("Expected updates['test']='value', got %v", updates["test"])
		}
	})

	t.Run("NoUpdates", func(t *testing.T) {
		builder := graph.NewUpdate()

		updates, err := builder.Build()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(updates) != 0 {
			t.Errorf("Expected empty updates, got %v", updates)
		}
	})
}

func TestUpdateBuilder_MustBuild(t *testing.T) {
	key := state.NewKey("test", "")
	builder := graph.NewUpdate()
	graph.UpdateSet(builder, key, "value")

	updates := builder.MustBuild()
	if updates["test"] != "value" {
		t.Errorf("Expected updates['test']='value', got %v", updates["test"])
	}
}

func TestUpdateBuilder_DuplicateKey(t *testing.T) {
	key := state.NewKey("test", "")
	builder := graph.NewUpdate()
	graph.UpdateSet(builder, key, "value1")
	graph.UpdateSet(builder, key, "value2") // Duplicate

	_, err := builder.Build()
	if err == nil {
		t.Fatal("Expected error for duplicate key, got nil")
	}
}

func TestUpdateBuilder_TypedOperations(t *testing.T) {
	counterKey := state.NewKey("counter", 0)
	messagesKey := state.NewListKey[string]("messages", 10)

	builder := graph.NewUpdate()
	graph.UpdateSet(builder, counterKey, 42)
	graph.UpdateAppend(builder, messagesKey, "msg1", "msg2")

	updates, err := builder.Build()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if updates["counter"] != 42 {
		t.Errorf("Expected counter=42, got %v", updates["counter"])
	}
}

func TestUpdateBuilder_IsEmpty(t *testing.T) {
	builder := graph.NewUpdate()
	if !builder.IsEmpty() {
		t.Error("Expected empty builder")
	}

	key := state.NewKey("test", "")
	graph.UpdateSet(builder, key, "value")
	if builder.IsEmpty() {
		t.Error("Expected non-empty builder")
	}
}
