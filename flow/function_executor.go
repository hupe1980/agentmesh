package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/metrics"
	"github.com/hupe1980/agentmesh/trace"
)

// Metric / tracing identifiers (deduplicated literals)
const (
	metricFunctionDuration = "agentmesh_function_duration_seconds"
	metricFunctionsTotal   = "agentmesh_functions_total"
	traceNamespace         = "agentmesh/agent"
)

// FunctionExecutor executes a batch of function/tool calls (possibly in parallel)
// and emits function response events through the provided callback.
type FunctionExecutor interface {
	Execute(
		ctx context.Context,
		reqCtx core.RequestContext,
		agent Agent,
		toolRegistry map[string]core.Tool,
		fnCalls []*core.FunctionCall,
		emit func(*core.Event) error,
	) error
}

// parallelFunctionExecutor is the default implementation.
type parallelFunctionExecutor struct {
	maxParallel int
}

// NewParallelFunctionExecutor creates a parallel executor. If maxParallel <= 0,
// it defaults to processing up to len(fnCalls) concurrently.
func NewParallelFunctionExecutor(maxParallel int) FunctionExecutor {
	return &parallelFunctionExecutor{maxParallel: maxParallel}
}

// buildFunctionResponseEvent executes a single FunctionCall with panic safety.
// Duration is recorded via metrics/tracing; logging of duration is removed.
func (e *parallelFunctionExecutor) buildFunctionResponseEvent(
	ctx context.Context,
	reqCtx core.RequestContext,
	agent Agent,
	toolRegistry map[string]core.Tool,
	fc *core.FunctionCall,
) (*core.Event, error) {
	tr := trace.FromContext(ctx).Tracer(traceNamespace)
	met := metrics.FromContext(ctx)
	log := logging.FromContext(ctx).With(
		"agent", agent.Name(),
		"function", fc.Name,
		"function_call_id", fc.ID,
	)

	ctx, span := tr.Start(ctx, "Function.Call",
		trace.Attr{Key: "agent.name", Value: agent.Name()},
		trace.Attr{Key: "function.name", Value: fc.Name},
		trace.Attr{Key: "function_call.id", Value: fc.ID},
	)
	start := time.Now()
	defer func() {
		met.Histogram(metricFunctionDuration).Record(ctx, time.Since(start).Seconds(),
			metrics.Attr{Key: "agent.name", Value: agent.Name()},
			metrics.Attr{Key: "function.name", Value: fc.Name},
		)
		span.End(nil)
	}()
	met.Counter(metricFunctionsTotal).Add(ctx, 1, metrics.Attr{Key: "agent.name", Value: agent.Name()})

	toolCtx := core.NewToolContext(reqCtx, func(o *core.ToolContextOptions) { o.FunctionCallID = core.String(fc.ID) })
	tool, ok := toolRegistry[fc.Name]
	if !ok {
		return nil, fmt.Errorf("%w: tool=%s", core.ErrToolNotFound, fc.Name)
	}

	argMap, argErr := parseFunctionArguments(fc.Arguments)
	if argErr != nil {
		return nil, argErr
	}

	log.Info("agent.function.start")

	result, err := e.executeToolWithPlugins(ctx, reqCtx, toolCtx, tool, argMap)
	if err != nil {
		return nil, err
	}

	ev := core.NewFunctionResponseEvent(reqCtx.RunID(), agent.Name(), fc.ID, fc.Name, result)
	ev.ApplyActions(toolCtx.EventActions())

	return ev, nil
}

// executeToolWithPlugins runs the Before/After/Error plugin pipeline around a tool call.
func (e *parallelFunctionExecutor) executeToolWithPlugins(
	ctx context.Context,
	reqCtx core.RequestContext,
	toolCtx core.ToolContext,
	tool core.Tool,
	argMap map[string]any,
) (any, error) {
	log := logging.FromContext(ctx)

	var final any

	overridden, err := reqCtx.RunBeforeTool(ctx, tool, toolCtx, argMap)
	if err != nil {
		log.Error("agent.function.before_tool.error", "error", err)
		return nil, err
	}

	if overridden != nil {
		final = overridden
	} else {
		var (
			callErr error
			result  any
		)

		func() {
			defer func() {
				if r := recover(); r != nil {
					callErr = panicError(r)
					log.Error("agent.function.tool.panic", "recover", r)
				}
			}()

			result, callErr = tool.Call(ctx, toolCtx, argMap)
		}()

		if callErr != nil {
			recovered, herr := reqCtx.RunOnToolError(ctx, tool, toolCtx, argMap, callErr)
			if herr != nil { // plugin error supersedes original
				log.Error("agent.function.error", "error", herr)
				return nil, herr
			}

			if recovered == nil { // unrecovered error
				return nil, callErr
			}

			final = recovered
		} else { // success path
			final = result
		}
	}

	modified, err := reqCtx.RunAfterTool(ctx, tool, toolCtx, argMap, final)
	if err != nil {
		log.Error("agent.function.after_tool.error", "error", err)
		return nil, err
	}

	if modified != nil {
		final = modified
	}

	return final, nil
}

// parseFunctionArguments converts the raw JSON argument string into a map for tool execution.
func parseFunctionArguments(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	var argMap map[string]any
	if err := json.Unmarshal([]byte(raw), &argMap); err != nil {
		return nil, fmt.Errorf("%w: %v", core.ErrInvalidToolArgs, err)
	}
	if argMap == nil { // ensure non-nil map for tool implementations
		argMap = map[string]any{}
	}
	return argMap, nil
}

// Execute runs a batch of FunctionCalls in parallel (or serially) respecting maxParallel and
// emits successful results as they complete (out-of-order). All calls are attempted; errors are
// aggregated (errors.Join) and returned after completion. Successful calls each emit exactly one
// event; failed calls emit none.
func (e *parallelFunctionExecutor) Execute(
	ctx context.Context,
	reqCtx core.RequestContext,
	agent Agent,
	toolRegistry map[string]core.Tool,
	fnCalls []*core.FunctionCall,
	emit func(*core.Event) error,
) error {
	n := len(fnCalls)
	if n == 0 {
		return nil
	}

	// Fast path: single call
	if n == 1 {
		if err := ctx.Err(); err != nil {
			return err
		}

		ev, err := e.buildFunctionResponseEvent(ctx, reqCtx, agent, toolRegistry, fnCalls[0])
		if err != nil {
			return err
		}

		if err := emit(ev); err != nil {
			return err
		}

		return nil
	}

	maxPar := e.maxParallel
	if maxPar <= 0 || maxPar > n {
		maxPar = n
	}

	var (
		wg    sync.WaitGroup
		sem   = make(chan struct{}, maxPar)
		errs  []error
		errMu sync.Mutex
	)

	addErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		errs = append(errs, err)
		errMu.Unlock()
	}

	for i, fc := range fnCalls {
		wg.Add(1)
		sem <- struct{}{}

		go func(_ int, call *core.FunctionCall) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := ctx.Err(); err != nil {
				addErr(fmt.Errorf("function %s (id=%s): canceled: %w", call.Name, call.ID, err))
				return
			}

			ev, err := e.buildFunctionResponseEvent(ctx, reqCtx, agent, toolRegistry, call)
			if err != nil {
				addErr(err)
				return
			}

			if ev == nil { // safety guard; should not happen if err==nil
				return
			}

			if emitErr := emit(ev); emitErr != nil {
				addErr(emitErr)
			}
		}(i, fc)
	}

	wg.Wait()
	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

// panicError converts a recovered panic to an error type
func panicError(r any) error {
	return &panicErr{val: r, stack: debug.Stack()}
}

// panicErr represents a recovered panic.
type panicErr struct {
	val   any
	stack []byte
}

// Error implements the error interface.
func (p *panicErr) Error() string { return "panic recovered" }
