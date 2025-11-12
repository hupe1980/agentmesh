package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// PersistencePlugin stores execution data to a database for audit and analytics.
// It requires a SQL database connection and creates the necessary tables on Init.
type PersistencePlugin struct {
	callbacks.NoopPlugin

	db *sql.DB
}

// NewPersistencePlugin creates a persistence plugin with the given database connection.
func NewPersistencePlugin(db *sql.DB) *PersistencePlugin {
	return &PersistencePlugin{db: db}
}

// Init creates database tables if they don't exist.
func (p *PersistencePlugin) Init(ctx context.Context) error {
	// Create tables if they don't exist
	schema := `
	CREATE TABLE IF NOT EXISTS graph_executions (
		id TEXT PRIMARY KEY,
		graph_id TEXT,
		start_time TIMESTAMP,
		end_time TIMESTAMP,
		status TEXT,
		error TEXT,
		nodes_visited INTEGER,
		duration_ms INTEGER
	);

	CREATE TABLE IF NOT EXISTS model_calls (
		id TEXT PRIMARY KEY,
		graph_id TEXT,
		timestamp TIMESTAMP,
		messages TEXT,
		system_prompt TEXT,
		response TEXT,
		error TEXT,
		duration_ms INTEGER
	);

	CREATE TABLE IF NOT EXISTS tool_calls (
		id TEXT PRIMARY KEY,
		graph_id TEXT,
		timestamp TIMESTAMP,
		tool_name TEXT,
		input TEXT,
		output TEXT,
		error TEXT,
		duration_ms INTEGER
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		graph_id TEXT,
		timestamp TIMESTAMP,
		type TEXT,
		content TEXT
	);
	`

	_, err := p.db.ExecContext(ctx, schema)
	return err
}

// OnGraphStart records graph execution start in database.
func (p *PersistencePlugin) OnGraphStart(ctx context.Context, graphID string) error {
	_, err := p.db.ExecContext(ctx,
		"INSERT INTO graph_executions (id, graph_id, start_time, status) VALUES (?, ?, ?, ?)",
		graphID, graphID, time.Now(), "running")
	return err
}

// OnGraphComplete records graph execution completion in database.
func (p *PersistencePlugin) OnGraphComplete(ctx context.Context, graphID string, stats callbacks.GraphStats) error {
	_, err := p.db.ExecContext(ctx,
		"UPDATE graph_executions SET end_time = ?, status = ?, nodes_visited = ?, duration_ms = ? WHERE id = ?",
		time.Now(), "completed", stats.NodesVisited, stats.Duration.Milliseconds(), graphID)
	return err
}

// OnGraphError records graph execution error in database.
func (p *PersistencePlugin) OnGraphError(ctx context.Context, graphID string, err error) error {
	_, dbErr := p.db.ExecContext(ctx,
		"UPDATE graph_executions SET end_time = ?, status = ?, error = ? WHERE id = ?",
		time.Now(), "failed", err.Error(), graphID)
	return dbErr
}

// AfterModel persists model invocation to database.
func (p *PersistencePlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	messagesJSON, _ := json.Marshal(req.Messages)
	responseJSON, _ := json.Marshal(resp)

	_, err := p.db.ExecContext(ctx,
		"INSERT INTO model_calls (id, timestamp, messages, system_prompt, response) VALUES (?, ?, ?, ?, ?)",
		generateID(), time.Now(), messagesJSON, req.SystemPrompt, responseJSON)

	return nil, err
}

// OnModelError persists model error to database.
func (p *PersistencePlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	messagesJSON, _ := json.Marshal(req.Messages)

	_, dbErr := p.db.ExecContext(ctx,
		"INSERT INTO model_calls (id, timestamp, messages, system_prompt, error) VALUES (?, ?, ?, ?, ?)",
		generateID(), time.Now(), messagesJSON, req.SystemPrompt, err.Error())

	return nil, dbErr
}

// AfterTool persists tool execution to database.
func (p *PersistencePlugin) AfterTool(ctx context.Context, toolName string, result callbacks.ToolResult) error {
	outputJSON, _ := json.Marshal(result.Output)

	_, err := p.db.ExecContext(ctx,
		"INSERT INTO tool_calls (id, timestamp, tool_name, output, duration_ms) VALUES (?, ?, ?, ?, ?)",
		generateID(), time.Now(), toolName, outputJSON, result.Duration.Milliseconds())

	return err
}

// OnToolError persists tool error to database.
func (p *PersistencePlugin) OnToolError(ctx context.Context, toolName string, err error) error {
	_, dbErr := p.db.ExecContext(ctx,
		"INSERT INTO tool_calls (id, timestamp, tool_name, error) VALUES (?, ?, ?, ?)",
		generateID(), time.Now(), toolName, err.Error())

	return dbErr
}

// OnMessage persists message to database.
func (p *PersistencePlugin) OnMessage(ctx context.Context, msg message.Message) error {
	content := message.Stringify(msg)

	_, err := p.db.ExecContext(ctx,
		"INSERT INTO messages (id, timestamp, type, content) VALUES (?, ?, ?, ?)",
		generateID(), time.Now(), string(msg.Type()), content)

	return err
}

// Shutdown closes the database connection.
func (p *PersistencePlugin) Shutdown(ctx context.Context) error {
	return p.db.Close()
}

// generateID generates a simple unique ID (in production, use UUID)
func generateID() string {
	return time.Now().Format("20060102150405.000000")
}
