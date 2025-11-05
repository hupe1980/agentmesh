package sql_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	sqlcp "github.com/hupe1980/agentmesh/pkg/checkpoint/sql"
	"github.com/hupe1980/agentmesh/pkg/message"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func TestSQLiteCheckpointer(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create checkpointer
	checkpointer, err := sqlcp.NewSQLiteCheckpointer(ctx, db)
	if err != nil {
		t.Fatalf("Failed to create checkpointer: %v", err)
	}
	defer checkpointer.Close()

	// Test Save
	cp := &checkpoint.Checkpoint{
		RunID:     "test-run-1",
		Superstep: 1,
		Timestamp: time.Now(),
		State: map[string]any{
			"counter": 42,
			"status":  "processing",
		},
		Messages:       []message.Message{},
		CompletedNodes: []string{"node1", "node2"},
		PausedNodes:    []string{},
		Metadata: map[string]any{
			"version": "1.0",
		},
	}

	if err := checkpointer.Save(ctx, cp); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Test Load
	loaded, err := checkpointer.Load(ctx, "test-run-1")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded checkpoint is nil")
	}

	if loaded.RunID != cp.RunID {
		t.Errorf("RunID mismatch: got %s, want %s", loaded.RunID, cp.RunID)
	}

	if loaded.Superstep != cp.Superstep {
		t.Errorf("Superstep mismatch: got %d, want %d", loaded.Superstep, cp.Superstep)
	}

	// Test List
	checkpoints, err := checkpointer.List(ctx, "test-run-1")
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		t.Errorf("Expected 1 checkpoint, got %d", len(checkpoints))
	}

	// Test LoadAtSuperstep
	atSuperstep, err := checkpointer.LoadAtSuperstep(ctx, "test-run-1", 1)
	if err != nil {
		t.Fatalf("Failed to load at superstep: %v", err)
	}

	if atSuperstep == nil {
		t.Fatal("Checkpoint at superstep is nil")
	}

	// Test Delete
	if err := checkpointer.Delete(ctx, "test-run-1"); err != nil {
		t.Fatalf("Failed to delete checkpoints: %v", err)
	}

	// Verify deletion
	deleted, err := checkpointer.Load(ctx, "test-run-1")
	if err != nil {
		t.Fatalf("Error loading after delete: %v", err)
	}

	if deleted != nil {
		t.Error("Expected nil after deletion")
	}
}

func TestSQLiteCheckpointer_MultipleSupersteps(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	checkpointer, err := sqlcp.NewSQLiteCheckpointer(ctx, db)
	if err != nil {
		t.Fatalf("Failed to create checkpointer: %v", err)
	}
	defer checkpointer.Close()

	// Save multiple checkpoints
	for i := int64(1); i <= 5; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     "multi-run",
			Superstep: i,
			Timestamp: time.Now(),
			State: map[string]any{
				"superstep": i,
			},
			Messages:       []message.Message{},
			CompletedNodes: []string{},
			PausedNodes:    []string{},
			Metadata:       map[string]any{},
		}

		if err := checkpointer.Save(ctx, cp); err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// Load should return most recent
	loaded, err := checkpointer.Load(ctx, "multi-run")
	if err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	if loaded.Superstep != 5 {
		t.Errorf("Expected superstep 5, got %d", loaded.Superstep)
	}

	// List should return all in descending order
	all, err := checkpointer.List(ctx, "multi-run")
	if err != nil {
		t.Fatalf("Failed to list: %v", err)
	}

	if len(all) != 5 {
		t.Errorf("Expected 5 checkpoints, got %d", len(all))
	}

	// Verify descending order
	for i, cp := range all {
		expectedSuperstep := int64(5 - i)
		if cp.Superstep != expectedSuperstep {
			t.Errorf("Checkpoint %d: expected superstep %d, got %d", i, expectedSuperstep, cp.Superstep)
		}
	}

	// Load specific superstep
	cp3, err := checkpointer.LoadAtSuperstep(ctx, "multi-run", 3)
	if err != nil {
		t.Fatalf("Failed to load superstep 3: %v", err)
	}

	if cp3.Superstep != 3 {
		t.Errorf("Expected superstep 3, got %d", cp3.Superstep)
	}
}
