package core

import "context"

// Plugin offers global interception points across agent, tool, and model
// execution so you can observe or alter behavior around well-defined stages.
//
// Unlike per-agent callbacks that target a single agent, a Plugin is
// registered once and affects every agent executed by the runner.
//
// Common applications include centralized logging/telemetry, metrics,
// caching, and request/response rewriting at lifecycle boundaries.
type Plugin interface {
	// OnUserParts runs when user parts are received, before the runner starts.
	//
	// Use this to log or modify the incoming parts. Returning a non-nil []Part replaces
	// the user parts; returning nil proceeds normally.
	OnUserParts(ctx context.Context, reqCtx RequestContext, userParts []Part) ([]Part, error)

	// BeforeRun runs before the runner starts.
	//
	// Use this for global setup or initialization. Returning a non-nil []Part halts execution
	// and ends the runner with that value; returning nil proceeds normally.
	BeforeRun(ctx context.Context, reqCtx RequestContext) ([]Part, error)

	// AfterRun runs after a runner invocation has completed.
	//
	// Use this for cleanup, final logging, or reporting. Return nil on success;
	// return an error to signal failure.
	AfterRun(ctx context.Context, reqCtx RequestContext) error

	// OnEvent runs after an event is produced by the runner but before it is
	// handed off to downstream consumers.
	//
	// Use this to inspect, log, redact, enrich, or transform events. Returning a
	// non-nil *Event replaces the original; returning nil leaves the original
	// event unchanged.
	OnEvent(ctx context.Context, reqCtx RequestContext, event *Event) (*Event, error)

	// BeforeAgent runs before an agent's primary logic (its Run / model invocation) is executed.
	//
	// Use this for logging, per‑agent setup, injecting messages, or short‑circuiting
	// execution. Returning a non-nil []Part bypasses the agent's normal processing
	// pipeline and those parts are treated as the agent's output. Returning nil allows
	// the agent to proceed normally.
	BeforeAgent(ctx context.Context, cbCtx CallbackContext, agent Agent) ([]Part, error)

	// AfterAgent runs after an agent's primary logic has completed.
	//
	// Use this to inspect, log, post‑process, or replace the agent's final
	// output. Returning a non-nil []Part replaces the original result. Returning
	// nil preserves the agent's original output.
	AfterAgent(ctx context.Context, cbCtx CallbackContext, agent Agent) ([]Part, error)

	// BeforeModel runs before a request is sent to the model.
	//
	// Use this to inspect, log, augment, or rewrite the pending *ModelRequest,
	// or to implement caching. Returning a non-nil *ModelResponse (with nil
	// error) is treated as a cache hit / short‑circuit that skips the actual
	// model invocation (implementation specific). Returning nil proceeds
	// normally. Return an error to abort execution.
	BeforeModel(ctx context.Context, cbCtx CallbackContext, req *ModelRequest) (*ModelResponse, error)

	// AfterModel runs after a response is received from the model.
	//
	// Use this to log raw model output, collect metrics (latency, token usage),
	// normalize or post‑process content, or apply safety filters. Returning a
	// non-nil *ModelResponse (with nil error) replaces the original pointer;
	// returning nil keeps the original. Return an error to signal failure and
	// abort downstream handling.
	AfterModel(ctx context.Context, cbCtx CallbackContext, res *ModelResponse) (*ModelResponse, error)

	// OnModelError runs when a model invocation returns an error.
	//
	// Use this to log failures, implement retries, fallback models, or synthesize
	// a graceful degradation response. Returning a non-nil *ModelResponse (with
	// nil error) replaces the failed call's output and suppresses the original
	// error. Returning nil with a nil error leaves behavior implementation‑defined;
	// returning a non-nil error propagates the failure.
	OnModelError(ctx context.Context, cbCtx CallbackContext, req *ModelRequest, err error) (*ModelResponse, error)

	// BeforeTool runs before a tool is executed.
	//
	// Use this for logging tool usage, input validation, mutation of arguments,
	// or short‑circuiting execution. Returning a non-nil map[string]any skips the
	// actual tool call and that value is treated as the synthetic tool result.
	// Returning nil continues with the original (possibly modified in-place)
	// arguments.
	BeforeTool(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string) (any, error)

	// AfterTool runs after a tool has executed successfully.
	//
	// Use this to inspect, log, or modify the tool's result. Returning a
	// non-nil map[string]any replaces the original result (enabling
	// post‑processing or normalization). Returning nil keeps the original
	// result unchanged.
	AfterTool(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string, result any) (any, error)

	// OnToolError runs when a tool invocation returns an error.
	//
	// Use this to log, transform, or recover from tool failures. Returning a
	// non-nil value signals an alternate "successful" tool output (the exact
	// interpretation is implementation specific). Returning nil means no
	// override and normal error handling proceeds.
	OnToolError(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string, err error) (any, error)
}

// PluginManager coordinates execution of Plugin hooks across a set of plugins.
type PluginManager interface {
	// Plugins returns the registered plugins in their original registration order.
	Plugins() []Plugin
	// RunOnUserParts runs the OnUserParts hook across all plugins and returns the first non-nil replacement.
	RunOnUserParts(ctx context.Context, reqCtx RequestContext, userParts []Part) ([]Part, error)

	// RunBeforeAgent executes BeforeAgent hooks; first non-nil []Part short-circuits.
	RunBeforeAgent(ctx context.Context, cbCtx CallbackContext, agent Agent) ([]Part, error)

	// RunAfterAgent executes AfterAgent hooks; first non-nil []Part replaces output.
	RunAfterAgent(ctx context.Context, cbCtx CallbackContext, agent Agent) ([]Part, error)

	// RunOnEvent executes the OnEvent hook across all plugins in order, allowing
	// each to inspect or modify the event. Implementations should pass the
	// (possibly replaced) event to subsequent plugins. A non-nil *Event returned
	// by a plugin replaces the current event. Errors abort processing.
	RunOnEvent(ctx context.Context, reqCtx RequestContext, event *Event) (*Event, error)

	// RunBeforeRun executes the BeforeRun hook across all plugins in order.
	RunBeforeRun(ctx context.Context, reqCtx RequestContext) ([]Part, error)

	// RunAfterRun executes the AfterRun hook across all plugins in order.
	RunAfterRun(ctx context.Context, reqCtx RequestContext) error

	// RunBeforeModel executes the BeforeModel hook; first non-nil *ModelResponse short-circuits model invocation.
	RunBeforeModel(ctx context.Context, cbCtx CallbackContext, req *ModelRequest) (*ModelResponse, error)

	// RunAfterModel executes the AfterModel hook; first non-nil *ModelResponse replaces the original.
	RunAfterModel(ctx context.Context, cbCtx CallbackContext, res *ModelResponse) (*ModelResponse, error)

	// RunOnModelError executes the OnModelError hook chain; first non-nil *ModelResponse indicates recovery.
	RunOnModelError(ctx context.Context, cbCtx CallbackContext, req *ModelRequest, err error) (*ModelResponse, error)

	// RunOnToolError executes the OnToolError hook across all plugins in order.
	RunOnToolError(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string, err error) (any, error)

	// RunBeforeTool executes the BeforeTool hook across all plugins in order.
	// It stops on the first non-nil override result or error.
	RunBeforeTool(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string) (any, error)

	// RunAfterTool executes the AfterTool hook across all plugins in order.
	// It stops on the first non-nil modified result or error.
	RunAfterTool(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string, result any) (any, error)
}

// manager coordinates execution of Plugin hooks across a set of plugins.
// It invokes each plugin in registration order and short-circuits when appropriate.
type pluginManager struct {
	plugins []Plugin
}

// NewPluginManager creates a plugin manager that executes hooks across the provided plugins in order.
func NewPluginManager(plugins ...Plugin) PluginManager {
	return &pluginManager{
		plugins: plugins,
	}
}

// Plugins returns the registered plugins in their original registration order.
func (m *pluginManager) Plugins() []Plugin {
	return m.plugins
}

// RunOnUserParts runs the OnUserParts hook across all plugins and returns the first non-nil replacement.
func (m *pluginManager) RunOnUserParts(
	ctx context.Context,
	reqCtx RequestContext,
	userParts []Part,
) ([]Part, error) {
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
func (m *pluginManager) RunOnEvent(
	ctx context.Context,
	reqCtx RequestContext,
	event *Event,
) (*Event, error) {
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
func (m *pluginManager) RunBeforeAgent(
	ctx context.Context,
	cbCtx CallbackContext,
	agent Agent,
) ([]Part, error) {
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
func (m *pluginManager) RunAfterAgent(
	ctx context.Context,
	cbCtx CallbackContext,
	agent Agent,
) ([]Part, error) {
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
func (m *pluginManager) RunBeforeRun(ctx context.Context, reqCtx RequestContext) ([]Part, error) {
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
func (m *pluginManager) RunAfterRun(ctx context.Context, reqCtx RequestContext) error {
	for _, plugin := range m.plugins {
		if err := plugin.AfterRun(ctx, reqCtx); err != nil {
			return err
		}
	}

	return nil
}

// RunOnToolError executes the OnToolError hook across all plugins in order.
// It stops on the first non-nil result or error.
func (m *pluginManager) RunOnToolError(
	ctx context.Context,
	tool Tool,
	toolCtx ToolContext,
	toolArgs string,
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
func (m *pluginManager) RunBeforeTool(
	ctx context.Context,
	tool Tool,
	toolCtx ToolContext,
	toolArgs string,
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
func (m *pluginManager) RunAfterTool(
	ctx context.Context,
	tool Tool,
	toolCtx ToolContext,
	toolArgs string,
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

// RunBeforeModel executes BeforeModel hooks; first non-nil *ModelResponse short-circuits.
func (m *pluginManager) RunBeforeModel(
	ctx context.Context,
	cbCtx CallbackContext,
	req *ModelRequest,
) (*ModelResponse, error) {
	for _, p := range m.plugins {
		out, err := p.BeforeModel(ctx, cbCtx, req)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, nil
}

// RunAfterModel executes AfterModel hooks; first non-nil *ModelResponse replaces original.
func (m *pluginManager) RunAfterModel(
	ctx context.Context,
	cbCtx CallbackContext,
	res *ModelResponse,
) (*ModelResponse, error) {
	cur := res
	for _, p := range m.plugins {
		out, err := p.AfterModel(ctx, cbCtx, cur)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, nil
}

// RunOnModelError executes OnModelError hooks; first non-nil *ModelResponse indicates recovery.
func (m *pluginManager) RunOnModelError(
	ctx context.Context,
	cbCtx CallbackContext,
	req *ModelRequest,
	err error,
) (*ModelResponse, error) {
	curErr := err
	for _, p := range m.plugins {
		out, hookErr := p.OnModelError(ctx, cbCtx, req, curErr)
		if hookErr != nil {
			return nil, hookErr
		}
		if out != nil {
			return out, nil
		}
	}
	return nil, curErr
}
