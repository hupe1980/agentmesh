package plugin

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/prompt"
)

// GlobalInstructions injects a shared instruction block before each model invocation.
// The provided instructions may be static or resolved dynamically through a provider.
type GlobalInstructions struct {
	*Noop
	instructions *core.Instructions
}

// NewGlobalInstructions constructs a plugin that prepends the given instructions to
// every model request handled during a run.
func NewGlobalInstructions(instructions *core.Instructions) *GlobalInstructions {
	return &GlobalInstructions{
		Noop:         NewNoop(),
		instructions: instructions,
	}
}

// BeforeModel resolves the configured instructions, renders templates against the
// session snapshot when available, and prepends the result to the model request.
func (pl *GlobalInstructions) BeforeModel(
	ctx context.Context,
	cbCtx core.CallbackContext,
	req *core.ModelRequest,
) (*core.ModelResponse, error) {
	if pl.instructions == nil {
		return nil, nil
	}

	instructions, err := pl.instructions.Resolve(ctx, cbCtx)
	if err != nil {
		return nil, err
	}

	if instructions == "" {
		return nil, nil
	}

	rendered := instructions
	snapshot := cbCtx.StateSnapshot()
	if len(snapshot) > 0 {
		tplOut, tplErr := prompt.Render(instructions, snapshot)
		if tplErr != nil {
			return nil, fmt.Errorf("failed to render template: %w", tplErr)
		}
		rendered = tplOut
	}

	if req.Instructions == "" {
		req.Instructions = rendered
	} else {
		req.Instructions = fmt.Sprintf("%s\n\n%s", rendered, req.Instructions)
	}

	return nil, nil
}

// Ensure Noop implements core.Plugin interface.
var _ core.Plugin = (*GlobalInstructions)(nil)
