// Package sql provides SQL-based checkpoint persistence using database/sql.
//
// The sql package implements the checkpoint.Checkpointer interface for SQL databases,
// supporting SQLite, PostgreSQL, MySQL, and MariaDB through a generic Dialect interface.
//
// # Basic Usage
//
//	db, _ := sql.Open("sqlite3", "checkpoints.db")
//	checkpointer, err := sql.NewSQLiteCheckpointer(ctx, db)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer checkpointer.Close()
//
//	// Use with a graph
//	g := graph.NewBuilder[MyState]().
//	    WithCheckpointer(checkpointer).
//	    Build()
//
// # Supported Databases
//
// SQLite:
//   - Driver: github.com/mattn/go-sqlite3
//   - Best for: Single-node deployments, development
//   - Features: File-based, zero configuration
//
// PostgreSQL:
//   - Driver: github.com/lib/pq or github.com/jackc/pgx/v5
//   - Best for: Production multi-node deployments
//   - Features: JSONB columns, advanced indexing, high concurrency
//
// MySQL/MariaDB:
//   - Driver: github.com/go-sql-driver/mysql
//   - Best for: High-performance workloads
//   - Features: JSON columns, InnoDB engine, replication support
//
// # Auto-Detection
//
// The checkpointer automatically detects the database dialect from the driver name:
//
//	// Automatically uses SQLite dialect
//	db, _ := sql.Open("sqlite3", "...")
//	checkpointer, _ := sql.NewCheckpointer(ctx, db)
//
//	// Automatically uses PostgreSQL dialect
//	db, _ := sql.Open("postgres", "...")
//	checkpointer, _ := sql.NewCheckpointer(ctx, db)
//
// # Manual Configuration
//
// For explicit control, use the database-specific constructors:
//
//	checkpointer, err := sql.NewSQLiteCheckpointer(ctx, db,
//	    sql.WithTableName("custom_checkpoints"),
//	)
//
//	checkpointer, err := sql.NewPostgreSQLCheckpointer(ctx, db,
//	    sql.WithTableName("graph_checkpoints"),
//	)
//
// # Table Schema
//
// The checkpointer creates a table with the following structure:
//   - id: Auto-incrementing primary key
//   - run_id: Graph execution run identifier
//   - superstep: Graph computation step number
//   - timestamp: Checkpoint creation time
//   - state: Serialized graph state including message history (JSON)
//   - completed_nodes: Array of completed node names for monitoring (JSON)
//   - paused_nodes: Array of paused node names for human-in-the-loop (JSON)
//   - metadata: Additional checkpoint metadata (JSON)
//
// Note: Message history is stored within the state column using the __messages__ key,
// not as a separate column. This ensures consistent state management.
//
// A unique constraint ensures only one checkpoint per (run_id, superstep) combination.
//
// # Custom Dialects
//
// For unsupported databases, implement the Dialect interface:
//
//	type MyDialect struct{}
//
//	func (d *MyDialect) CreateTableSQL(tableName string) string {
//	    return "CREATE TABLE IF NOT EXISTS ..."
//	}
//
//	func (d *MyDialect) PlaceholderForPosition(position int) string {
//	    return "?" // or "$1" style
//	}
//
//	checkpointer, err := sql.NewCheckpointer(ctx, db,
//	    sql.WithDialect(&MyDialect{}),
//	)
package sql
