package plugin

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// NoopPlugin provides default no-op implementations for all Plugin hooks.
// Embed this in your plugin struct to only override the hooks you need.
//
// Example:
//
//	type MetricsPlugin struct {
//	    plugin.NoopPlugin
//	    registry prometheus.Registry
//	}
//
//	func (p *MetricsPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
//	    p.registry.IncrementCounter("model_requests_total")
//	    return nil, nil
//	}
type NoopPlugin struct{}

// Init implements Plugin.Init as a no-op.
func (NoopPlugin) Init(ctx context.Context) error { return nil }

// Shutdown implements Plugin.Shutdown as a no-op.
func (NoopPlugin) Shutdown(ctx context.Context) error { return nil }

// BeforeNode implements Plugin.BeforeNode as a no-op.
func (NoopPlugin) BeforeNode(ctx context.Context, nodeName string, view state.ReadView) ([]string, state.Updates, error) {
	return nil, nil, nil
}

// AfterNode implements Plugin.AfterNode as a no-op.
func (NoopPlugin) AfterNode(ctx context.Context, nodeName string, view state.ReadView, updates state.Updates) error {
	return nil
}

// OnNodeError implements Plugin.OnNodeError as a no-op.
func (NoopPlugin) OnNodeError(ctx context.Context, nodeName string, err error) error { return nil }

// BeforeModel implements Plugin.BeforeModel as a no-op.
func (NoopPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	return nil, nil
}

// AfterModel implements Plugin.AfterModel as a no-op.
func (NoopPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	return nil, nil
}

// OnModelError implements Plugin.OnModelError as a no-op.
func (NoopPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	return nil, nil
}

// BeforeTool implements Plugin.BeforeTool as a no-op.
func (NoopPlugin) BeforeTool(ctx context.Context, toolName string, input any) error { return nil }

// AfterTool implements Plugin.AfterTool as a no-op.
func (NoopPlugin) AfterTool(ctx context.Context, toolName string, result any) error {
	return nil
}

// OnToolError implements Plugin.OnToolError as a no-op.
func (NoopPlugin) OnToolError(ctx context.Context, toolName string, err error) error { return nil }

// OnStateChange implements Plugin.OnStateChange as a no-op.
func (NoopPlugin) OnStateChange(ctx context.Context, nodeName string, updates state.Updates) error {
	return nil
}
