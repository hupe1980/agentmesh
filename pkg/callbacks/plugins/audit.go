package plugins

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
)

// AuditPlugin tracks all graph state changes for compliance and security auditing.
// It writes audit entries to the provided writer (e.g., file, network stream).
type AuditPlugin struct {
	callbacks.NoopPlugin

	writer io.Writer
	mu     sync.Mutex
}

// NewAuditPlugin creates an audit logging plugin.
// writer is where audit entries will be written (e.g., os.Stdout, file, network).
func NewAuditPlugin(writer io.Writer) *AuditPlugin {
	return &AuditPlugin{
		writer: writer,
	}
}

// OnStateChange writes state change events to the audit log.
func (p *AuditPlugin) OnStateChange(ctx context.Context, changes callbacks.StateChanges) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: "state_change",
		Changes:   changes,
		UserID:    getUserIDFromContext(ctx),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = p.writer.Write(append(data, '\n'))
	return err
}

// OnGraphStart writes graph start event to the audit log.
func (p *AuditPlugin) OnGraphStart(ctx context.Context, graphID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: "graph_start",
		GraphID:   graphID,
		UserID:    getUserIDFromContext(ctx),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = p.writer.Write(append(data, '\n'))
	return err
}

// OnGraphComplete writes graph completion event to the audit log.
func (p *AuditPlugin) OnGraphComplete(ctx context.Context, graphID string, stats callbacks.GraphStats) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: "graph_complete",
		GraphID:   graphID,
		Stats:     &stats,
		UserID:    getUserIDFromContext(ctx),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = p.writer.Write(append(data, '\n'))
	return err
}

// OnGraphError writes graph error event to the audit log.
func (p *AuditPlugin) OnGraphError(ctx context.Context, graphID string, err error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: "graph_error",
		GraphID:   graphID,
		Error:     err.Error(),
		UserID:    getUserIDFromContext(ctx),
	}

	data, _ := json.Marshal(entry)
	_, writeErr := p.writer.Write(append(data, '\n'))

	if writeErr != nil {
		return writeErr
	}

	return nil
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"`
	GraphID   string                 `json:"graph_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Changes   callbacks.StateChanges `json:"changes,omitempty"`
	Stats     *callbacks.GraphStats  `json:"stats,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// getUserIDFromContext extracts user ID from context (customize based on your auth system).
func getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return "unknown"
}
