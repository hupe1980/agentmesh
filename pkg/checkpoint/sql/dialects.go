package sql

import "fmt"

// SQLiteDialect provides SQLite-specific SQL syntax.
type SQLiteDialect struct{}

// CreateTableSQL generates the CREATE TABLE statement for SQLite.
func (d *SQLiteDialect) CreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			superstep INTEGER NOT NULL,
			timestamp DATETIME NOT NULL,
			state TEXT NOT NULL,
			messages TEXT NOT NULL,
			completed_nodes TEXT NOT NULL,
			paused_nodes TEXT NOT NULL,
			metadata TEXT,
			UNIQUE(run_id, superstep)
		)
	`, tableName)
}

// PlaceholderForPosition returns the SQL placeholder for SQLite.
func (d *SQLiteDialect) PlaceholderForPosition(position int) string {
	return "?"
}

// PostgreSQLDialect provides PostgreSQL-specific SQL syntax.
type PostgreSQLDialect struct{}

// CreateTableSQL generates the CREATE TABLE statement for PostgreSQL.
func (d *PostgreSQLDialect) CreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			run_id TEXT NOT NULL,
			superstep BIGINT NOT NULL,
			timestamp TIMESTAMP NOT NULL,
			state JSONB NOT NULL,
			messages JSONB NOT NULL,
			completed_nodes JSONB NOT NULL,
			paused_nodes JSONB NOT NULL,
			metadata JSONB,
			UNIQUE(run_id, superstep)
		)
	`, tableName)
}

// PlaceholderForPosition returns the SQL placeholder for PostgreSQL.
func (d *PostgreSQLDialect) PlaceholderForPosition(position int) string {
	return fmt.Sprintf("$%d", position)
}

// MySQLDialect provides MySQL/MariaDB-specific SQL syntax.
type MySQLDialect struct{}

// CreateTableSQL generates the CREATE TABLE statement for MySQL.
func (d *MySQLDialect) CreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			run_id VARCHAR(255) NOT NULL,
			superstep BIGINT NOT NULL,
			timestamp DATETIME NOT NULL,
			state JSON NOT NULL,
			messages JSON NOT NULL,
			completed_nodes JSON NOT NULL,
			paused_nodes JSON NOT NULL,
			metadata JSON,
			UNIQUE KEY unique_run_superstep (run_id, superstep),
			INDEX idx_run_id (run_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`, tableName)
}

// PlaceholderForPosition returns the SQL placeholder for MySQL.
func (d *MySQLDialect) PlaceholderForPosition(position int) string {
	return "?"
}
