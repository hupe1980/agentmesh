package plugins

import (
	"context"
	"log"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// LoggingPlugin logs all lifecycle events for debugging and monitoring.
// It embeds NoopPlugin to inherit default no-op implementations.
type LoggingPlugin struct {
	plugin.NoopPlugin
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

// BeforeNode logs before node execution.
func (p *LoggingPlugin) BeforeNode(ctx context.Context, nodeName string, view *state.ReadView) (state.Updates, error) {
	p.logger.Printf("%s Before node: %s", p.prefix, nodeName)
	return nil, nil // No short-circuit
}

// AfterNode logs after node execution.
func (p *LoggingPlugin) AfterNode(ctx context.Context, nodeName string, view *state.ReadView, updates state.Updates) error {
	p.logger.Printf("%s After node: %s", p.prefix, nodeName)
	return nil
}

// OnNodeError logs node execution errors.
func (p *LoggingPlugin) OnNodeError(ctx context.Context, nodeName string, err error) error {
	p.logger.Printf("%s Node error: %s - %v", p.prefix, nodeName, err)
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
func (p *LoggingPlugin) AfterTool(ctx context.Context, toolName string, result any) error {
	p.logger.Printf("%s After tool: %s", p.prefix, toolName)
	return nil
}

// OnToolError logs tool execution errors.
func (p *LoggingPlugin) OnToolError(ctx context.Context, toolName string, err error) error {
	p.logger.Printf("%s Tool error: %s - %v", p.prefix, toolName, err)
	return nil
}

// OnStateChange logs state changes.
func (p *LoggingPlugin) OnStateChange(ctx context.Context, nodeName string, updates state.Updates) error {
	p.logger.Printf("%s State changed by %s: %d keys modified",
		p.prefix, nodeName, len(updates))
	return nil
}
