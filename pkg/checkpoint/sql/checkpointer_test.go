package sql_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	sqlcp "github.com/hupe1980/agentmesh/pkg/checkpoint/sql"
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

func TestTableNameValidation(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		tableName string
		wantError bool
		errorMsg  string
	}{
		// Valid table names
		{
			name:      "valid_default",
			tableName: "checkpoints",
			wantError: false,
		},
		{
			name:      "valid_underscores",
			tableName: "my_checkpoints_table",
			wantError: false,
		},
		{
			name:      "valid_alphanumeric",
			tableName: "checkpoints123",
			wantError: false,
		},
		{
			name:      "valid_starts_with_underscore",
			tableName: "_checkpoints",
			wantError: false,
		},
		{
			name:      "valid_mixed_case",
			tableName: "MyCheckpoints",
			wantError: false,
		},
		// Invalid table names - SQL injection attempts
		{
			name:      "injection_drop_table",
			tableName: "checkpoints; DROP TABLE users--",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "injection_union_select",
			tableName: "checkpoints UNION SELECT * FROM users",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "injection_comment",
			tableName: "checkpoints--",
			wantError: true,
			errorMsg:  "SQL comment",
		},
		{
			name:      "injection_single_quote",
			tableName: "checkpoint'; DROP TABLE users--",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "injection_double_quote",
			tableName: `checkpoint"; DROP TABLE users--`,
			wantError: true,
			errorMsg:  "invalid table name",
		},
		// Invalid table names - special characters
		{
			name:      "invalid_hyphens",
			tableName: "my-checkpoints",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "invalid_spaces",
			tableName: "my checkpoints",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "invalid_special_chars",
			tableName: "check@points",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "invalid_dot",
			tableName: "my.checkpoints",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "invalid_backslash",
			tableName: "check\\points",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		// Invalid table names - SQL keywords
		{
			name:      "keyword_select",
			tableName: "SELECT",
			wantError: true,
			errorMsg:  "SQL keyword",
		},
		{
			name:      "keyword_drop",
			tableName: "DROP",
			wantError: true,
			errorMsg:  "SQL keyword",
		},
		{
			name:      "keyword_insert",
			tableName: "INSERT",
			wantError: true,
			errorMsg:  "SQL keyword",
		},
		{
			name:      "keyword_delete",
			tableName: "DELETE",
			wantError: true,
			errorMsg:  "SQL keyword",
		},
		{
			name:      "keyword_table",
			tableName: "TABLE",
			wantError: true,
			errorMsg:  "SQL keyword",
		},
		// Invalid table names - length/format
		{
			name:      "empty_string",
			tableName: "",
			wantError: true,
			errorMsg:  "cannot be empty",
		},
		{
			name:      "starts_with_number",
			tableName: "123checkpoints",
			wantError: true,
			errorMsg:  "invalid table name",
		},
		{
			name:      "too_long",
			tableName: "this_is_a_very_long_table_name_that_exceeds_the_maximum_length_limit_of_64_characters",
			wantError: true,
			errorMsg:  "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sqlcp.NewCheckpointer(ctx, db, sqlcp.WithTableName(tt.tableName))

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for table name %q, got nil", tt.tableName)
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for table name %q, got %v", tt.tableName, err)
				}
			}
		})
	}
}

func TestSQLInjectionPrevention(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create a test table to verify injection doesn't work
	_, err = db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Try to create checkpointer with malicious table name
	maliciousNames := []string{
		"checkpoints; DROP TABLE users--",
		"checkpoints' OR '1'='1",
		"checkpoints UNION SELECT * FROM users",
		"checkpoints\"; DROP TABLE users--",
	}

	for _, name := range maliciousNames {
		t.Run("injection_"+name, func(t *testing.T) {
			_, err := sqlcp.NewCheckpointer(ctx, db, sqlcp.WithTableName(name))
			if err == nil {
				t.Errorf("Expected error for malicious table name %q, got nil", name)
			}
		})
	}

	// Verify the users table still exists and wasn't dropped
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query sqlite_master: %v", err)
	}

	if count != 1 {
		t.Error("users table was affected by injection attempt")
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
