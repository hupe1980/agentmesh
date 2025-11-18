package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Checkpointer implements checkpoint.Checkpointer using database/sql.
// It's database-agnostic and works with any SQL driver (SQLite, PostgreSQL, MySQL, MariaDB, etc.)
type Checkpointer struct {
	db        *sql.DB
	tableName string
	dialect   Dialect
}

// Dialect defines database-specific SQL syntax variations.
type Dialect interface {
	// CreateTableSQL returns the SQL statement to create the checkpoints table.
	CreateTableSQL(tableName string) string

	// PlaceholderForPosition returns the placeholder syntax for the given position.
	// Examples: "?" for MySQL/SQLite, "$1" for PostgreSQL
	PlaceholderForPosition(position int) string
}

// Option configures Checkpointer.
type Option func(*Checkpointer)

// WithTableName sets a custom table name (default: "checkpoints").
func WithTableName(name string) Option {
	return func(c *Checkpointer) {
		c.tableName = name
	}
}

// WithDialect sets a custom SQL dialect (default: auto-detected from driver).
func WithDialect(dialect Dialect) Option {
	return func(c *Checkpointer) {
		c.dialect = dialect
	}
}

// NewCheckpointer creates a new SQL-based checkpointer.
// It automatically creates the checkpoints table if it doesn't exist.
//
// Example:
//
//	db, _ := sql.Open("sqlite3", "checkpoints.db")
//	checkpointer, err := sql.NewCheckpointer(ctx, db)
func NewCheckpointer(ctx context.Context, db *sql.DB, opts ...Option) (*Checkpointer, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	c := &Checkpointer{
		db:        db,
		tableName: "checkpoints",
	}

	for _, opt := range opts {
		opt(c)
	}

	// Auto-detect dialect if not set
	if c.dialect == nil {
		driverName := ""
		if driver := db.Driver(); driver != nil {
			// Try to get driver name (not standardized, but often available)
			driverName = fmt.Sprintf("%T", driver)
		}

		c.dialect = detectDialect(driverName)
	}

	// Create table if not exists
	if err := c.createTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to create checkpoints table: %w", err)
	}

	return c, nil
}

func detectDialect(driverName string) Dialect {
	// Simple heuristic based on driver type name
	switch {
	case strings.Contains(driverName, "postgres"), strings.Contains(driverName, "pgx"):
		return &PostgreSQLDialect{}
	case strings.Contains(driverName, "mysql"), strings.Contains(driverName, "mariadb"):
		return &MySQLDialect{}
	case strings.Contains(driverName, "sqlite"):
		return &SQLiteDialect{}
	default:
		// Default to SQLite syntax (most compatible)
		return &SQLiteDialect{}
	}
}

func (c *Checkpointer) createTable(ctx context.Context) error {
	createSQL := c.dialect.CreateTableSQL(c.tableName)
	_, err := c.db.ExecContext(ctx, createSQL)
	return err
}

// Save persists a checkpoint to the database.
func (c *Checkpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if cp.RunID == "" {
		return fmt.Errorf("checkpoint RunID is empty")
	}

	// Serialize complex fields to JSON
	stateJSON, err := json.Marshal(cp.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	completedNodesJSON, err := json.Marshal(cp.CompletedNodes)
	if err != nil {
		return fmt.Errorf("failed to marshal completed nodes: %w", err)
	}

	pausedNodesJSON, err := json.Marshal(cp.PausedNodes)
	if err != nil {
		return fmt.Errorf("failed to marshal paused nodes: %w", err)
	}

	metadataJSON, err := json.Marshal(cp.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Insert checkpoint
	// Note: messages column may still exist in schema for backward compatibility,
	// but message history is now stored in state via MessagesKey
	//nolint:gosec // Table name is sanitized, placeholders used for values
	insertSQL := fmt.Sprintf(`
		INSERT INTO %s (
			run_id, superstep, timestamp, 
			state, completed_nodes, paused_nodes, metadata
		) VALUES (%s, %s, %s, %s, %s, %s, %s)
	`, c.tableName,
		c.dialect.PlaceholderForPosition(1),
		c.dialect.PlaceholderForPosition(2),
		c.dialect.PlaceholderForPosition(3),
		c.dialect.PlaceholderForPosition(4),
		c.dialect.PlaceholderForPosition(5),
		c.dialect.PlaceholderForPosition(6),
		c.dialect.PlaceholderForPosition(7),
	)

	_, err = c.db.ExecContext(ctx, insertSQL,
		cp.RunID,
		cp.Superstep,
		cp.Timestamp,
		stateJSON,
		completedNodesJSON,
		pausedNodesJSON,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// Load retrieves the most recent checkpoint for a run ID.
func (c *Checkpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
	}

	//nolint:gosec // Table name is sanitized, placeholders used for values
	querySQL := fmt.Sprintf(`
		SELECT 
			run_id, superstep, timestamp,
			state, completed_nodes, paused_nodes, metadata
		FROM %s
		WHERE run_id = %s
		ORDER BY superstep DESC
		LIMIT 1
	`, c.tableName, c.dialect.PlaceholderForPosition(1))

	var (
		stateJSON          []byte
		completedNodesJSON []byte
		pausedNodesJSON    []byte
		metadataJSON       []byte
	)

	cp := &checkpoint.Checkpoint{}

	err := c.db.QueryRowContext(ctx, querySQL, runID).Scan(
		&cp.RunID,
		&cp.Superstep,
		&cp.Timestamp,
		&stateJSON,
		&completedNodesJSON,
		&pausedNodesJSON,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No checkpoint found (not an error)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Deserialize JSON fields
	if err := json.Unmarshal(stateJSON, &cp.State); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	if err := json.Unmarshal(completedNodesJSON, &cp.CompletedNodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal completed nodes: %w", err)
	}
	if err := json.Unmarshal(pausedNodesJSON, &cp.PausedNodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal paused nodes: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, &cp.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return cp, nil
}

// List returns all checkpoints for a run ID, ordered by superstep (newest first).
func (c *Checkpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
	}

	//nolint:gosec // Table name is sanitized, placeholders used for values
	querySQL := fmt.Sprintf(`
		SELECT 
			run_id, superstep, timestamp,
			state, completed_nodes, paused_nodes, metadata
		FROM %s
		WHERE run_id = %s
		ORDER BY superstep DESC
	`, c.tableName, c.dialect.PlaceholderForPosition(1))

	rows, err := c.db.QueryContext(ctx, querySQL, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}
	defer func() {
		_ = rows.Close() // Error on Close() is not critical for read operations
	}()

	var checkpoints []*checkpoint.Checkpoint

	for rows.Next() {
		var (
			stateJSON          []byte
			completedNodesJSON []byte
			pausedNodesJSON    []byte
			metadataJSON       []byte
		)

		cp := &checkpoint.Checkpoint{}

		if err := rows.Scan(
			&cp.RunID,
			&cp.Superstep,
			&cp.Timestamp,
			&stateJSON,
			&completedNodesJSON,
			&pausedNodesJSON,
			&metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
		}

		// Deserialize JSON fields
		if err := json.Unmarshal(stateJSON, &cp.State); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}
		if err := json.Unmarshal(completedNodesJSON, &cp.CompletedNodes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal completed nodes: %w", err)
		}
		if err := json.Unmarshal(pausedNodesJSON, &cp.PausedNodes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal paused nodes: %w", err)
		}
		if err := json.Unmarshal(metadataJSON, &cp.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		checkpoints = append(checkpoints, cp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating checkpoints: %w", err)
	}

	return checkpoints, nil
}

// Delete removes all checkpoints for a run ID.
func (c *Checkpointer) Delete(ctx context.Context, runID string) error {
	if runID == "" {
		return fmt.Errorf("runID is empty")
	}

	//nolint:gosec // Table name is sanitized, placeholders used for values
	deleteSQL := fmt.Sprintf(`
		DELETE FROM %s WHERE run_id = %s
	`, c.tableName, c.dialect.PlaceholderForPosition(1))

	result, err := c.db.ExecContext(ctx, deleteSQL, runID)
	if err != nil {
		return fmt.Errorf("failed to delete checkpoints: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no checkpoints found for runID: %s", runID)
	}

	return nil
}

// LoadAtSuperstep retrieves a checkpoint at a specific superstep.
func (c *Checkpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
	}

	//nolint:gosec // Table name is sanitized, placeholders used for values
	querySQL := fmt.Sprintf(`
		SELECT 
			run_id, superstep, timestamp,
			state, completed_nodes, paused_nodes, metadata
		FROM %s
		WHERE run_id = %s AND superstep = %s
		LIMIT 1
	`, c.tableName,
		c.dialect.PlaceholderForPosition(1),
		c.dialect.PlaceholderForPosition(2))

	var (
		stateJSON          []byte
		completedNodesJSON []byte
		pausedNodesJSON    []byte
		metadataJSON       []byte
	)

	cp := &checkpoint.Checkpoint{}

	err := c.db.QueryRowContext(ctx, querySQL, runID, superstep).Scan(
		&cp.RunID,
		&cp.Superstep,
		&cp.Timestamp,
		&stateJSON,
		&completedNodesJSON,
		&pausedNodesJSON,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No checkpoint found at this superstep
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Deserialize JSON fields
	if err := json.Unmarshal(stateJSON, &cp.State); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	if err := json.Unmarshal(completedNodesJSON, &cp.CompletedNodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal completed nodes: %w", err)
	}
	if err := json.Unmarshal(pausedNodesJSON, &cp.PausedNodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal paused nodes: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, &cp.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return cp, nil
}

// Close closes the database connection.
func (c *Checkpointer) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}
