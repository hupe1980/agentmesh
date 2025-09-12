package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// ExecuteAgent runs an agent with BeforeAgent / AfterAgent hook semantics.
//
// Lifecycle:
//  1. BeforeAgent: if it returns a non-nil []Part, the agent's Run is skipped and
//     those parts are emitted as a synthetic assistant event (short-circuit). AfterAgent still runs.
//  2. Agent Run (only if not short-circuited) emits its normal events directly to the provided writer.
//  3. AfterAgent: if it returns a non-nil []Part, a new assistant event is appended
//     (it does not mutate or retract earlier output).
//
// History is strictly append-only; no prior events are modified or removed.
func ExecuteAgent(ctx context.Context, reqCtx core.RequestContext, ag core.Agent, w core.EventWriter) error {
	// BeforeAgent short-circuit path
	if parts, err := reqCtx.RunBeforeAgent(ctx, ag); err != nil {
		return fmt.Errorf("plugin: before_agent: %w", err)
	} else if parts != nil {
		// Emit synthetic assistant event
		assist := core.NewFullAssistantEvent(reqCtx.RunID(), reqCtx.AgentName(), parts...)
		if err := w.Write(ctx, assist); err != nil {
			return fmt.Errorf("failed to write synthetic assistant event: %w", err)
		}

		// Skip normal agent Run, but still run AfterAgent
		return runAfterAgent(ctx, reqCtx, ag, w)
	}

	// Normal execution
	if err := ag.Run(ctx, reqCtx, w); err != nil {
		return err
	}

	return runAfterAgent(ctx, reqCtx, ag, w)
}

// runAfterAgent invokes the AfterAgent plugin hook and, if parts are returned,
// appends a new assistant event. Returns any error encountered.
func runAfterAgent(ctx context.Context, reqCtx core.RequestContext, ag core.Agent, w core.EventWriter) error {
	if afterParts, err := reqCtx.RunAfterAgent(ctx, ag); err != nil {
		return fmt.Errorf("plugin: after_agent: %w", err)
	} else if afterParts != nil {
		repl := core.NewFullAssistantEvent(reqCtx.RunID(), reqCtx.AgentName(), afterParts...)
		if err := w.Write(ctx, repl); err != nil {
			return fmt.Errorf("failed to write after_agent replacement event: %w", err)
		}
	}

	return nil
}
