package sql

import (
	"context"
	"database/sql"
)

// NewSQLiteCheckpointer creates a SQLite-based checkpointer.
func NewSQLiteCheckpointer(ctx context.Context, db *sql.DB, opts ...Option) (*Checkpointer, error) {
	opts = append(opts, WithDialect(&SQLiteDialect{}))
	return NewCheckpointer(ctx, db, opts...)
}

// NewPostgreSQLCheckpointer creates a PostgreSQL-based checkpointer.
func NewPostgreSQLCheckpointer(ctx context.Context, db *sql.DB, opts ...Option) (*Checkpointer, error) {
	opts = append(opts, WithDialect(&PostgreSQLDialect{}))
	return NewCheckpointer(ctx, db, opts...)
}

// NewMySQLCheckpointer creates a MySQL/MariaDB-based checkpointer.
func NewMySQLCheckpointer(ctx context.Context, db *sql.DB, opts ...Option) (*Checkpointer, error) {
	opts = append(opts, WithDialect(&MySQLDialect{}))
	return NewCheckpointer(ctx, db, opts...)
}
