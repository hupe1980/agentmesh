package plugin

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// manager coordinates execution of Plugin hooks across a set of plugins.
// It invokes each plugin in registration order and short-circuits when appropriate.
type manager struct {
	plugins []core.Plugin
}

// NewManager creates a plugin manager that executes hooks across the provided plugins in order.
func NewManager(plugins ...core.Plugin) core.PluginManager {
	return &manager{
		plugins: plugins,
	}
}

// RunOnUserParts runs the OnUserParts hook across all plugins and returns the first non-nil replacement.
func (m *manager) RunOnUserParts(
	ctx context.Context,
	reqCtx core.RequestContext,
	userParts []core.Part,
) ([]core.Part, error) {
	var current = userParts
	for _, plugin := range m.plugins {
		out, err := plugin.OnUserParts(ctx, reqCtx, current)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}

	return nil, nil
}

// RunOnEvent executes OnEvent across plugins sequentially, feeding the (possibly
// replaced) event into the next plugin. Returns final replacement (if any).
func (m *manager) RunOnEvent(
	ctx context.Context,
	reqCtx core.RequestContext,
	event *core.Event,
) (*core.Event, error) {
	cur := event
	for _, p := range m.plugins {
		out, err := p.OnEvent(ctx, reqCtx, cur)
		if err != nil {
			return nil, err
		}
		if out != nil {
			cur = out
		}
	}
	if cur != event {
		return cur, nil
	}
	return nil, nil
}

// RunBeforeAgent executes BeforeAgent hooks until one returns non-nil parts.
func (m *manager) RunBeforeAgent(
	ctx context.Context,
	cbCtx core.CallbackContext,
	agent core.Agent,
) ([]core.Part, error) {
	for _, p := range m.plugins {
		out, err := p.BeforeAgent(ctx, cbCtx, agent)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, nil
}

// RunAfterAgent executes AfterAgent hooks until one returns non-nil parts.
func (m *manager) RunAfterAgent(
	ctx context.Context,
	cbCtx core.CallbackContext,
	agent core.Agent,
) ([]core.Part, error) {
	for _, p := range m.plugins {
		out, err := p.AfterAgent(ctx, cbCtx, agent)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, nil
}

// RunBeforeRun executes the BeforeRun hook across all plugins in order.
// If any plugin returns a non-nil []Part, it short-circuits and returns it.
func (m *manager) RunBeforeRun(ctx context.Context, reqCtx core.RequestContext) ([]core.Part, error) {
	for _, plugin := range m.plugins {
		out, err := plugin.BeforeRun(ctx, reqCtx)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}

	return nil, nil
}

// RunAfterRun executes the AfterRun hook across all plugins in order.
// It stops on the first error encountered.
func (m *manager) RunAfterRun(ctx context.Context, reqCtx core.RequestContext) error {
	for _, plugin := range m.plugins {
		if err := plugin.AfterRun(ctx, reqCtx); err != nil {
			return err
		}
	}

	return nil
}

// RunOnToolError executes the OnToolError hook across all plugins in order.
// It stops on the first non-nil result or error.
func (m *manager) RunOnToolError(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs map[string]any,
	err error,
) (any, error) {
	var currentErr = err
	for _, plugin := range m.plugins {
		out, err := plugin.OnToolError(ctx, tool, toolCtx, toolArgs, currentErr)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}

	return nil, currentErr
}

// RunBeforeTool executes the BeforeTool hook across all plugins in order.
// It stops on the first non-nil override or error.
func (m *manager) RunBeforeTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs map[string]any,
) (any, error) {
	for _, plugin := range m.plugins {
		out, err := plugin.BeforeTool(ctx, tool, toolCtx, toolArgs)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, nil
}

// RunAfterTool executes the AfterTool hook across all plugins in order.
// It stops on the first non-nil modified result or error.
func (m *manager) RunAfterTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs map[string]any,
	result any,
) (any, error) {
	current := result
	for _, plugin := range m.plugins {
		out, err := plugin.AfterTool(ctx, tool, toolCtx, toolArgs, current)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, nil
}

// Interface compliance (compile-time assertions)
var _ core.PluginManager = (*manager)(nil)
