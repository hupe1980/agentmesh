package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

var (
	// validTableNameRegex allows only alphanumeric characters and underscores.
	// Table names must start with a letter or underscore.
	// Hyphens are NOT allowed as they require quoting in SQL and can cause ambiguity.
	validTableNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)
)

// sanitizeTableName validates and sanitizes a table name to prevent SQL injection.
// Returns an error if the table name contains invalid characters or is too long.
//
// Security rules:
// - Must start with letter or underscore
// - Can only contain alphanumeric characters and underscores
// - Maximum length of 64 characters (compatible with MySQL/PostgreSQL/SQLite limits)
// - Rejects SQL keywords, special characters, and comment sequences
// - No hyphens (they require quoting and can cause SQL ambiguity)
func sanitizeTableName(name string) (string, error) {
	if err := validate.NotEmpty(name, "table name"); err != nil {
		return "", err
	}

	// Check length (most databases have 64-character limit)
	if len(name) > 64 {
		return "", fmt.Errorf("table name too long (max 64 characters): %s", name)
	}

	// Reject SQL comment sequences (defense-in-depth)
	if strings.Contains(name, "--") || strings.Contains(name, "/*") || strings.Contains(name, "*/") {
		return "", fmt.Errorf("invalid table name: cannot contain SQL comment sequences: %s", name)
	}

	// Validate against allowlist pattern
	if !validTableNameRegex.MatchString(name) {
		return "", fmt.Errorf("invalid table name: must start with letter or underscore and contain only alphanumeric and underscore characters: %s", name)
	}

	// Reject SQL keywords (case-insensitive)
	upperName := strings.ToUpper(name)
	sqlKeywords := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER",
		"TABLE", "FROM", "WHERE", "JOIN", "UNION", "ORDER", "GROUP", "HAVING",
	}
	for _, keyword := range sqlKeywords {
		if upperName == keyword {
			return "", fmt.Errorf("table name cannot be SQL keyword: %s", name)
		}
	}

	return name, nil
}

// Checkpointer implements checkpoint.Checkpointer using database/sql.
// It's database-agnostic and works with any SQL driver (SQLite, PostgreSQL, MySQL, MariaDB, etc.)
//
// Security: Table names are validated to prevent SQL injection. User-provided table names
// are sanitized using an allowlist approach that only permits safe characters.
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
// The table name is validated to prevent SQL injection attacks.
// Returns an error if the table name contains invalid characters.
//
// Valid table names:
// - Must start with a letter or underscore
// - Can contain alphanumeric characters and underscores only
// - Maximum 64 characters
// - Cannot be SQL keywords or contain SQL comment sequences
//
// Example valid names: "checkpoints", "my_checkpoints", "agent_state"
// Example invalid names: "my table", "drop;table", "123start", "my-table"
func WithTableName(name string) Option {
	return func(c *Checkpointer) {
		// Note: Validation happens in NewCheckpointer, but we store the name here
		// This allows the error to be returned during construction
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
// Security: Table names are validated to prevent SQL injection. Custom table names
// must contain only alphanumeric characters, underscores, and hyphens.
//
// Example:
//
//	db, _ := sql.Open("sqlite3", "checkpoints.db")
//	checkpointer, err := sql.NewCheckpointer(ctx, db)
//
//	// With custom table name
//	checkpointer, err := sql.NewCheckpointer(ctx, db, sql.WithTableName("my_checkpoints"))
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

	// Validate and sanitize table name to prevent SQL injection
	sanitizedName, err := sanitizeTableName(c.tableName)
	if err != nil {
		return nil, fmt.Errorf("invalid table name: %w", err)
	}
	c.tableName = sanitizedName

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

// ListPendingApprovals returns all checkpoints with pending approvals.
func (c *Checkpointer) ListPendingApprovals(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	//nolint:gosec // Table name is sanitized
	query := fmt.Sprintf(`
		SELECT run_id, superstep, timestamp, version, state, pending_writes, 
		       completed_nodes, paused_nodes, metadata, committed, approval_metadata, signature
		FROM %s
		WHERE approval_metadata IS NOT NULL
		ORDER BY timestamp DESC
	`, c.tableName)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending approvals: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close rows: %w", closeErr)
		}
	}()

	var checkpoints []*checkpoint.Checkpoint
	for rows.Next() {
		cp, scanErr := c.scanCheckpointRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}

		// Only include checkpoints with pending approvals
		if cp.ApprovalMetadata != nil && len(cp.ApprovalMetadata.PendingApprovals) > 0 {
			checkpoints = append(checkpoints, cp)
		}
	}

	return checkpoints, rows.Err()
}

// scanCheckpointRow scans a checkpoint row from SQL result set.
func (c *Checkpointer) scanCheckpointRow(rows *sql.Rows) (*checkpoint.Checkpoint, error) {
	var (
		stateJSON            []byte
		pendingWritesJSON    []byte
		completedNodesJSON   []byte
		pausedNodesJSON      []byte
		metadataJSON         []byte
		approvalMetadataJSON []byte
		signatureBytes       []byte
	)

	cp := &checkpoint.Checkpoint{}

	if err := rows.Scan(
		&cp.RunID,
		&cp.Superstep,
		&cp.Timestamp,
		&cp.Version,
		&stateJSON,
		&pendingWritesJSON,
		&completedNodesJSON,
		&pausedNodesJSON,
		&metadataJSON,
		&cp.Committed,
		&approvalMetadataJSON,
		&signatureBytes,
	); err != nil {
		return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
	}

	// Deserialize JSON fields
	if err := json.Unmarshal(stateJSON, &cp.State); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	if len(pendingWritesJSON) > 0 {
		if err := json.Unmarshal(pendingWritesJSON, &cp.PendingWrites); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pending writes: %w", err)
		}
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
	if len(approvalMetadataJSON) > 0 {
		cp.ApprovalMetadata = &checkpoint.ApprovalMetadata{}
		if err := json.Unmarshal(approvalMetadataJSON, cp.ApprovalMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal approval metadata: %w", err)
		}
	}
	if len(signatureBytes) > 0 {
		cp.Signature = signatureBytes
	}

	return cp, nil
}

// GetApprovalHistory returns the approval history for a specific run.
func (c *Checkpointer) GetApprovalHistory(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error) {
	//nolint:gosec // Table name is sanitized at construction time
	query := fmt.Sprintf("SELECT approval_metadata FROM %s WHERE run_id = %s AND approval_metadata IS NOT NULL ORDER BY superstep ASC",
		c.tableName,
		c.dialect.PlaceholderForPosition(1))

	rows, err := c.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query approval history: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close rows: %w", closeErr)
		}
	}()

	var history []checkpoint.ApprovalRecord
	for rows.Next() {
		var approvalMetadataJSON []byte
		if err := rows.Scan(&approvalMetadataJSON); err != nil {
			return nil, fmt.Errorf("failed to scan approval metadata: %w", err)
		}

		if len(approvalMetadataJSON) == 0 {
			continue
		}

		var metadata checkpoint.ApprovalMetadata
		if err := json.Unmarshal(approvalMetadataJSON, &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal approval metadata: %w", err)
		}

		history = append(history, metadata.ApprovalHistory...)
	}

	return history, rows.Err()
}
