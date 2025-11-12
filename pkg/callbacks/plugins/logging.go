package plugins

import (
	"context"
	"log"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// LoggingPlugin logs all lifecycle events for debugging and monitoring.
// It embeds NoopPlugin to inherit default no-op implementations.
type LoggingPlugin struct {
	callbacks.NoopPlugin
	logger *log.Logger
	prefix string
}

// NewLoggingPlugin creates a new logging plugin.
// prefix is prepended to all log messages (e.g., "[AgentMesh]").
func NewLoggingPlugin(logger *log.Logger, prefix string) *LoggingPlugin {
	if prefix == "" {
		prefix = "[Plugin]"
	}
	return &LoggingPlugin{
		logger: logger,
		prefix: prefix,
	}
}

// Init initializes the plugin.
func (p *LoggingPlugin) Init(ctx context.Context) error {
	p.logger.Printf("%s Init", p.prefix)
	return nil
}

// Shutdown cleans up plugin resources.
func (p *LoggingPlugin) Shutdown(ctx context.Context) error {
	p.logger.Printf("%s Shutdown", p.prefix)
	return nil
}

// OnGraphStart logs when graph execution begins.
func (p *LoggingPlugin) OnGraphStart(ctx context.Context, graphID string) error {
	p.logger.Printf("%s Graph started: %s", p.prefix, graphID)
	return nil
}

// OnGraphComplete logs when graph execution completes.
func (p *LoggingPlugin) OnGraphComplete(ctx context.Context, graphID string, stats callbacks.GraphStats) error {
	p.logger.Printf("%s Graph completed: %s (duration: %v, nodes: %d)",
		p.prefix, graphID, stats.Duration, stats.NodesVisited)
	return nil
}

// OnGraphError logs when graph execution fails.
func (p *LoggingPlugin) OnGraphError(ctx context.Context, graphID string, err error) error {
	p.logger.Printf("%s Graph error: %s - %v", p.prefix, graphID, err)
	return nil
}

// BeforeNode logs before node execution.
func (p *LoggingPlugin) BeforeNode(ctx context.Context, nodeName string) error {
	p.logger.Printf("%s Before node: %s", p.prefix, nodeName)
	return nil
}

// AfterNode logs after node execution.
func (p *LoggingPlugin) AfterNode(ctx context.Context, nodeName string, result callbacks.NodeResult) error {
	p.logger.Printf("%s After node: %s (duration: %v)", p.prefix, nodeName, result.Duration)
	return nil
}

// BeforeModel logs before model invocation.
func (p *LoggingPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	p.logger.Printf("%s Before model (messages: %d, tools: %d)",
		p.prefix, len(req.Messages), len(req.Tools))
	return nil, nil // No short-circuit
}

// AfterModel logs after model invocation.
func (p *LoggingPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	p.logger.Printf("%s After model (response message added)", p.prefix)
	return nil, nil // No transformation
}

// OnModelError logs model invocation errors.
func (p *LoggingPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	p.logger.Printf("%s Model error: %v", p.prefix, err)
	return nil, nil // No fallback
}

// BeforeTool logs before tool execution.
func (p *LoggingPlugin) BeforeTool(ctx context.Context, toolName string, input any) error {
	p.logger.Printf("%s Before tool: %s", p.prefix, toolName)
	return nil
}

// AfterTool logs after tool execution.
func (p *LoggingPlugin) AfterTool(ctx context.Context, toolName string, result callbacks.ToolResult) error {
	p.logger.Printf("%s After tool: %s (duration: %v)", p.prefix, toolName, result.Duration)
	return nil
}

// OnToolError logs tool execution errors.
func (p *LoggingPlugin) OnToolError(ctx context.Context, toolName string, err error) error {
	p.logger.Printf("%s Tool error: %s - %v", p.prefix, toolName, err)
	return nil
}

// OnStateChange logs state changes.
func (p *LoggingPlugin) OnStateChange(ctx context.Context, changes callbacks.StateChanges) error {
	p.logger.Printf("%s State change (added: %d, updated: %d, removed: %d)",
		p.prefix, len(changes.Added), len(changes.Updated), len(changes.Removed))
	return nil
}

// OnMessage logs message emissions.
func (p *LoggingPlugin) OnMessage(ctx context.Context, msg message.Message) error {
	p.logger.Printf("%s Message added: %s", p.prefix, msg.Type())
	return nil
}
