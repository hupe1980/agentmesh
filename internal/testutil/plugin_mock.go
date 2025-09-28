package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// PluginMock is a function-based mock implementing core.Plugin.
// Each function field is optional; if left nil the method is a no-op returning neutral values.
type PluginMock struct {
	OnUserPartsFunc  func(context.Context, core.RequestContext, []core.Part) ([]core.Part, error)
	BeforeRunFunc    func(context.Context, core.RequestContext) ([]core.Part, error)
	AfterRunFunc     func(context.Context, core.RequestContext) error
	OnEventFunc      func(context.Context, core.RequestContext, *core.Event) (*core.Event, error)
	BeforeAgentFunc  func(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error)
	AfterAgentFunc   func(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error)
	BeforeModelFunc  func(context.Context, core.CallbackContext, *core.ModelRequest) (*core.ModelResponse, error)
	AfterModelFunc   func(context.Context, core.CallbackContext, *core.ModelResponse) (*core.ModelResponse, error)
	OnModelErrorFunc func(context.Context, core.CallbackContext, *core.ModelRequest, error) (*core.ModelResponse, error)
	BeforeToolFunc   func(context.Context, core.Tool, core.ToolContext, string) (any, error)
	AfterToolFunc    func(context.Context, core.Tool, core.ToolContext, string, any) (any, error)
	OnToolErrorFunc  func(context.Context, core.Tool, core.ToolContext, string, error) (any, error)
}

func (m *PluginMock) OnUserParts(ctx context.Context, r core.RequestContext, p []core.Part) ([]core.Part, error) {
	if m.OnUserPartsFunc != nil {
		return m.OnUserPartsFunc(ctx, r, p)
	}
	return nil, nil
}
func (m *PluginMock) BeforeRun(ctx context.Context, r core.RequestContext) ([]core.Part, error) {
	if m.BeforeRunFunc != nil {
		return m.BeforeRunFunc(ctx, r)
	}
	return nil, nil
}
func (m *PluginMock) AfterRun(ctx context.Context, r core.RequestContext) error {
	if m.AfterRunFunc != nil {
		return m.AfterRunFunc(ctx, r)
	}
	return nil
}
func (m *PluginMock) OnEvent(ctx context.Context, r core.RequestContext, e *core.Event) (*core.Event, error) {
	if m.OnEventFunc != nil {
		return m.OnEventFunc(ctx, r, e)
	}
	return nil, nil
}
func (m *PluginMock) BeforeAgent(ctx context.Context, cb core.CallbackContext, a core.Agent) ([]core.Part, error) {
	if m.BeforeAgentFunc != nil {
		return m.BeforeAgentFunc(ctx, cb, a)
	}
	return nil, nil
}
func (m *PluginMock) AfterAgent(ctx context.Context, cb core.CallbackContext, a core.Agent) ([]core.Part, error) {
	if m.AfterAgentFunc != nil {
		return m.AfterAgentFunc(ctx, cb, a)
	}
	return nil, nil
}
func (m *PluginMock) BeforeModel(
	ctx context.Context,
	cb core.CallbackContext,
	req *core.ModelRequest,
) (*core.ModelResponse, error) {
	if m.BeforeModelFunc != nil {
		return m.BeforeModelFunc(ctx, cb, req)
	}
	return nil, nil
}
func (m *PluginMock) AfterModel(
	ctx context.Context,
	cb core.CallbackContext,
	res *core.ModelResponse,
) (*core.ModelResponse, error) {
	if m.AfterModelFunc != nil {
		return m.AfterModelFunc(ctx, cb, res)
	}
	return nil, nil
}
func (m *PluginMock) OnModelError(
	ctx context.Context,
	cb core.CallbackContext,
	req *core.ModelRequest,
	err error,
) (*core.ModelResponse, error) {
	if m.OnModelErrorFunc != nil {
		return m.OnModelErrorFunc(ctx, cb, req, err)
	}
	return nil, nil
}
func (m *PluginMock) BeforeTool(
	ctx context.Context,
	t core.Tool,
	tc core.ToolContext,
	args string,
) (any, error) {
	if m.BeforeToolFunc != nil {
		return m.BeforeToolFunc(ctx, t, tc, args)
	}
	return nil, nil
}
func (m *PluginMock) AfterTool(
	ctx context.Context,
	t core.Tool,
	tc core.ToolContext,
	args string,
	result any,
) (any, error) {
	if m.AfterToolFunc != nil {
		return m.AfterToolFunc(ctx, t, tc, args, result)
	}
	return nil, nil
}
func (m *PluginMock) OnToolError(
	ctx context.Context,
	t core.Tool,
	tc core.ToolContext,
	args string,
	err error,
) (any, error) {
	if m.OnToolErrorFunc != nil {
		return m.OnToolErrorFunc(ctx, t, tc, args, err)
	}
	return nil, nil
}

var _ core.Plugin = (*PluginMock)(nil)
