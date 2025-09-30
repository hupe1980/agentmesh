package executor

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// ExecuteModel coordinates a single model invocation with plugin hook semantics.
// Order:
//  1. RunBeforeModel: if non-nil *ModelResponse returned, short-circuit generation.
//  2. Model.Generate streaming: partial chunks emitted immediately.
//  3. On model error: RunOnModelError; a non-nil *ModelResponse indicates recovery.
//  4. RunAfterModel: post-process final response (replacement allowed) and emit.
//
// This mirrors the logic used by flow-level executors, exposed here as a reusable function.
func ExecuteModel(
	ctx context.Context,
	reqCtx core.RequestContext,
	m core.Model,
	req *core.ModelRequest,
) (<-chan *core.ModelResponse, <-chan error) {
	outCh := make(chan *core.ModelResponse)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		var sc *core.ModelResponse
		if pm := reqCtx.PluginManager(); pm != nil {
			cbCtx := core.NewCallbackContext(reqCtx)
			out, err := pm.RunBeforeModel(ctx, cbCtx, req)
			if err != nil {
				errCh <- fmt.Errorf("plugin: before_model: %w", err)
				return
			}
			sc = out
		}
		if sc != nil {
			if _, err := emitFinal(ctx, reqCtx, sc, outCh); err != nil {
				errCh <- err
			}
			return
		}

		respCh, genErrCh := m.Generate(ctx, req)

		var final *core.ModelResponse

	loop:
		for respCh != nil || genErrCh != nil {
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
					outCh <- r
					continue
				}

				final = r
			case err, ok := <-genErrCh:
				if ok {
					if pm := reqCtx.PluginManager(); pm != nil {
						cbCtx := core.NewCallbackContext(reqCtx)
						if rec, hookErr := pm.RunOnModelError(ctx, cbCtx, req, err); hookErr != nil {
							errCh <- fmt.Errorf("plugin: on_model_error: %w", hookErr)
							return
						} else if rec != nil {
							final = rec
							break loop
						}
					}
					errCh <- err
					return
				}
				genErrCh = nil
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}

			if respCh == nil && genErrCh == nil {
				break
			}
		}

		if final == nil {
			return
		}

		if pm := reqCtx.PluginManager(); pm != nil {
			cbCtx := core.NewCallbackContext(reqCtx)
			if rep, err := pm.RunAfterModel(ctx, cbCtx, final); err != nil {
				errCh <- fmt.Errorf("plugin: after_model: %w", err)
				return
			} else if rep != nil {
				final = rep
			}
		}

		if _, err := emitFinal(ctx, reqCtx, final, outCh); err != nil {
			errCh <- err
		}
	}()

	return outCh, errCh
}

// emitFinal runs AfterModel hook (allowing replacement) then emits the full response on outCh.
func emitFinal(
	_ context.Context,
	_ core.RequestContext,
	res *core.ModelResponse,
	outCh chan<- *core.ModelResponse,
) (*core.ModelResponse, error) {
	// Mark as non-partial to signal completion semantics downstream.
	res.Partial = false
	outCh <- res
	return res, nil
}

// DefaultModelExecutor is the reusable core.ModelExecutor implementation
// using ExecuteModel. Inject this wherever a ModelExecutor is required.
var DefaultModelExecutor core.ModelExecutor = core.ModelExecutorFunc(ExecuteModel)

// Compile-time assertion for the function adapter variable.
var _ core.ModelExecutor = DefaultModelExecutor
