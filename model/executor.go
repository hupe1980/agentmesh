package model

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
	beforeCallbacks []core.BeforeModelCallback,
	afterCallbacks []core.AfterModelCallback,
	m core.Model,
	req *core.ModelRequest,
) (<-chan *core.ModelResponse, <-chan error) {
	outCh := make(chan *core.ModelResponse)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		resp, err := handleBeforeModelCallbacks(ctx, reqCtx, beforeCallbacks, req)
		if err != nil {
			errCh <- err
			return
		}
		if resp != nil {
			if _, err := emitFinal(ctx, reqCtx, resp, outCh); err != nil {
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

		altered, err := handleAfterModelCallbacks(ctx, reqCtx, afterCallbacks, final)
		if err != nil {
			errCh <- err
			return
		}
		if altered != nil {
			final = altered
		}

		if _, err := emitFinal(ctx, reqCtx, final, outCh); err != nil {
			errCh <- err
		}
	}()

	return outCh, errCh
}

func handleBeforeModelCallbacks(
	ctx context.Context,
	reqCtx core.RequestContext,
	beforeCallbacks []core.BeforeModelCallback,
	req *core.ModelRequest,
) (*core.ModelResponse, error) {
	cbCtx := core.NewCallbackContext(reqCtx)

	if pm := reqCtx.PluginManager(); pm != nil {
		out, err := pm.RunBeforeModel(ctx, cbCtx, req)
		if err != nil {
			return nil, fmt.Errorf("plugin: before_model: %w", err)
		}

		if out != nil {
			return out, nil
		}
	}

	if len(beforeCallbacks) == 0 {
		return nil, nil
	}

	for i, cb := range beforeCallbacks {
		out, err := cb(ctx, cbCtx, req)
		if err != nil {
			return nil, fmt.Errorf("agent before_model callback %d failed: %w", i, err)
		}

		if out != nil {
			return out, nil
		}
	}

	return nil, nil
}

func handleAfterModelCallbacks(
	ctx context.Context,
	reqCtx core.RequestContext,
	afterCallbacks []core.AfterModelCallback,
	resp *core.ModelResponse,
) (*core.ModelResponse, error) {
	cbCtx := core.NewCallbackContext(reqCtx)

	if pm := reqCtx.PluginManager(); pm != nil {
		out, err := pm.RunAfterModel(ctx, cbCtx, resp)
		if err != nil {
			return nil, fmt.Errorf("plugin: after_model: %w", err)
		}

		if out != nil {
			return out, nil
		}
	}

	if len(afterCallbacks) == 0 {
		return nil, nil
	}

	for i, cb := range afterCallbacks {
		out, err := cb(ctx, cbCtx, resp)
		if err != nil {
			return nil, fmt.Errorf("agent after_model callback %d failed: %w", i, err)
		}

		if out != nil {
			return out, nil
		}
	}

	return nil, nil
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
