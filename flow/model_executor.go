package flow

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// ExecuteModel coordinates a single model invocation with plugin hook semantics.
// Order:
//  1. RunBeforeModel: if non-nil *ModelResponse returned, short-circuit generation.
//  2. Model.Generate streaming: partial chunks emitted immediately as partial events.
//  3. On model error: RunOnModelError; a non-nil *ModelResponse indicates recovery.
//  4. AfterModel: post-process final (or recovered / short-circuit) response; replacement allowed.
//
// Emitted events:
//   - Partial chunks -> PartialAssistant events
//   - Final chunk    -> FullAssistant event
//
// Note: Flows may apply additional request/response processors around this executor.
func ExecuteModel(
	ctx context.Context,
	requestCtx core.RequestContext,
	agent Agent,
	req *core.ModelRequest,
	writer core.EventWriter,
) (*core.ModelResponse, error) {
	// BeforeModel short-circuit
	if sc, err := requestCtx.RunBeforeModel(ctx, req); err != nil {
		return nil, fmt.Errorf("plugin: before_model: %w", err)
	} else if sc != nil { // allow AfterModel replacement before emission
		return emitFinal(ctx, requestCtx, writer, sc)
	}

	mdl := agent.Model()
	respCh, errCh := mdl.Generate(ctx, req)
	var final *core.ModelResponse

loop:
	for respCh != nil || errCh != nil {
		select {
		case r, ok := <-respCh:
			if !ok {
				respCh = nil
				continue
			}
			if r == nil {
				continue
			}
			if r.Partial {
				pev := core.NewPartialAssistantEvent(requestCtx.RunID(), requestCtx.AgentName(), r.Parts...)
				if err := writer.Write(ctx, pev); err != nil {
					return nil, fmt.Errorf("failed to write partial model event: %w", err)
				}
				continue
			}
			final = r
		case err, ok := <-errCh:
			if ok {
				if rec, hookErr := requestCtx.RunOnModelError(ctx, req, err); hookErr != nil {
					return nil, fmt.Errorf("plugin: on_model_error: %w", hookErr)
				} else if rec != nil {
					final = rec
					break loop
				}
				return nil, err
			}
			errCh = nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if respCh == nil && errCh == nil {
			break
		}
	}

	if final == nil {
		return nil, nil
	}

	return emitFinal(ctx, requestCtx, writer, final)
}

// emitFinal runs AfterModel (allowing replacement) and writes the final assistant event.
func emitFinal(
	ctx context.Context,
	requestCtx core.RequestContext,
	writer core.EventWriter,
	res *core.ModelResponse,
) (*core.ModelResponse, error) {
	if rep, err := requestCtx.RunAfterModel(ctx, res); err != nil {
		return nil, fmt.Errorf("plugin: after_model: %w", err)
	} else if rep != nil {
		res = rep
	}

	fev := core.NewFullAssistantEvent(requestCtx.RunID(), requestCtx.AgentName(), res.Parts...)
	if err := writer.Write(ctx, fev); err != nil {
		return nil, fmt.Errorf("failed to write final model event: %w", err)
	}

	return res, nil
}
