package plugin

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// Noop is a core.Plugin that performs no mutations and never short-circuits.
// All hooks return nil to indicate "proceed as normal".
type Noop struct{}

// NewNoop returns a no-op plugin.
func NewNoop() *Noop { return &Noop{} }

// OnUserParts returns nil to proceed without modifying user parts.
func (pl *Noop) OnUserParts(
	ctx context.Context,
	reqCtx core.RequestContext,
	userParts []core.Part,
) ([]core.Part, error) {
	return nil, nil
}

// BeforeRun returns nil to proceed normally.
func (pl *Noop) BeforeRun(ctx context.Context, reqCtx core.RequestContext) ([]core.Part, error) {
	return nil, nil
}

// AfterRun returns nil to indicate success.
func (pl *Noop) AfterRun(ctx context.Context, reqCtx core.RequestContext) error {
	return nil
}

// OnEvent returns nil to leave the original event unchanged.
func (pl *Noop) OnEvent(ctx context.Context, reqCtx core.RequestContext, event *core.Event) (*core.Event, error) {
	return nil, nil
}

// BeforeAgent returns nil to allow the agent to proceed normally.
func (pl *Noop) BeforeAgent(ctx context.Context, cbCtx core.CallbackContext, agent core.Agent) ([]core.Part, error) {
	return nil, nil
}

// AfterAgent returns nil to preserve the agent's original output.
func (pl *Noop) AfterAgent(ctx context.Context, cbCtx core.CallbackContext, agent core.Agent) ([]core.Part, error) {
	return nil, nil
}

// BeforeModel returns nil to proceed with the actual model invocation.
func (pl *Noop) BeforeModel(
	ctx context.Context,
	cbCtx core.CallbackContext,
	req *core.ModelRequest,
) (*core.ModelResponse, error) {
	return nil, nil
}

// AfterModel returns nil to keep the original model response.
func (pl *Noop) AfterModel(
	ctx context.Context,
	cbCtx core.CallbackContext,
	res *core.ModelResponse,
) (*core.ModelResponse, error) {
	return nil, nil
}

// OnModelError returns nil to not override error handling.
func (pl *Noop) OnModelError(
	ctx context.Context,
	cbCtx core.CallbackContext,
	req *core.ModelRequest,
	err error,
) (*core.ModelResponse, error) {
	return nil, nil
}

// BeforeTool returns nil to continue with the tool execution unchanged.
func (pl *Noop) BeforeTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs string,
) (any, error) {
	return nil, nil
}

// AfterTool returns nil to keep the original tool result.
func (pl *Noop) AfterTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs string,
	result any,
) (any, error) {
	return nil, nil
}

// OnToolError returns nil to not override tool error handling.
func (pl *Noop) OnToolError(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs string,
	err error,
) (any, error) {
	return nil, nil
}

// Ensure Noop implements core.Plugin interface.
var _ core.Plugin = (*Noop)(nil)
