package tool

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

// Metric / tracing identifiers
const (
	metricFunctionDuration = "agentmesh_function_duration_seconds"
	metricFunctionsTotal   = "agentmesh_functions_total"
	traceNamespace         = "agentmesh/agent"
)

// parallelToolExecutor is the default ToolExecutor implementation executing tool calls (possibly in parallel).
type parallelToolExecutor struct {
	maxParallel int
}

// NewParallelToolExecutor creates a new ToolExecutor respecting maxParallel concurrency.
// If maxParallel <= 0, it defaults to len(fnCalls) at execution time.
func NewParallelToolExecutor(maxParallel int) core.ToolExecutor { // returns core.ToolExecutor for integration
	return &parallelToolExecutor{maxParallel: maxParallel}
}

// Execute runs a batch of FunctionCalls, returning events in completion order and aggregating errors.
func (e *parallelToolExecutor) Execute(
	ctx context.Context,
	reqCtx core.RequestContext,
	toolRegistry map[string]core.Tool,
	fnCalls []*core.FunctionCall,
) ([]*core.Event, error) {
	tr := trace.FromContext(ctx).Tracer(traceNamespace)
	met := metrics.FromContext(ctx)
	log := logging.FromContext(ctx).With("agent", reqCtx.AgentName())

	ctx, span := tr.Start(ctx, "Functions.Batch",
		trace.Attr{Key: "agent.name", Value: reqCtx.AgentName()},
		trace.Attr{Key: "functions.count", Value: fmt.Sprintf("%d", len(fnCalls))},
	)

	batchStart := time.Now()

	defer func() {
		met.Histogram("agentmesh_functions_batch_duration_seconds").Record(
			ctx,
			time.Since(batchStart).Seconds(),
			metrics.Attr{Key: "agent.name", Value: reqCtx.AgentName()},
			metrics.Attr{Key: "functions.count", Value: fmt.Sprintf("%d", len(fnCalls))},
		)
		span.End(nil)
	}()

	met.Counter("agentmesh_functions_batches_total").Add(
		ctx,
		1,
		metrics.Attr{Key: "agent.name", Value: reqCtx.AgentName()},
	)

	met.Histogram("agentmesh_functions_batch_size").Record(
		ctx,
		float64(len(fnCalls)),
		metrics.Attr{Key: "agent.name", Value: reqCtx.AgentName()},
	)

	log.Info("agent.functions.batch.start", "count", len(fnCalls))

	n := len(fnCalls)
	if n == 0 {
		return nil, nil
	}

	if n == 1 { // fast path
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ev, err := e.buildToolEvent(ctx, reqCtx, toolRegistry, fnCalls[0])
		if err != nil {
			return nil, err
		}

		return []*core.Event{ev}, nil
	}

	maxPar := e.maxParallel
	if maxPar <= 0 || maxPar > n {
		maxPar = n
	}

	var (
		wg     sync.WaitGroup
		sem    = make(chan struct{}, maxPar)
		errs   []error
		errMu  sync.Mutex
		events = make([]*core.Event, 0, n)
		evMu   sync.Mutex
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

			Ev, err := e.buildToolEvent(ctx, reqCtx, toolRegistry, call)
			if err != nil {
				addErr(err)
				return
			}

			if Ev == nil {
				return
			}

			evMu.Lock()
			events = append(events, Ev)
			evMu.Unlock()
		}(i, fc)
	}

	wg.Wait()

	if len(errs) == 0 {
		return events, nil
	}

	return events, errors.Join(errs...)
}

// buildToolEvent executes a single tool (FunctionCall) with panic safety and plugin hooks.
func (e *parallelToolExecutor) buildToolEvent(
	ctx context.Context,
	reqCtx core.RequestContext,
	toolRegistry map[string]core.Tool,
	fc *core.FunctionCall,
) (*core.Event, error) {
	tr := trace.FromContext(ctx).Tracer(traceNamespace)
	met := metrics.FromContext(ctx)
	log := logging.FromContext(ctx).With(
		"agent", reqCtx.AgentName(),
		"function", fc.Name,
		"function_call_id", fc.ID,
	)

	ctx, span := tr.Start(ctx, "Function.Call",
		trace.Attr{Key: "agent.name", Value: reqCtx.AgentName()},
		trace.Attr{Key: "function.name", Value: fc.Name},
		trace.Attr{Key: "function_call.id", Value: fc.ID},
	)

	start := time.Now()

	defer func() {
		met.Histogram(metricFunctionDuration).Record(ctx, time.Since(start).Seconds(),
			metrics.Attr{Key: "agent.name", Value: reqCtx.AgentName()},
			metrics.Attr{Key: "function.name", Value: fc.Name},
		)
		span.End(nil)
	}()

	met.Counter(metricFunctionsTotal).Add(ctx, 1, metrics.Attr{Key: "agent.name", Value: reqCtx.AgentName()})

	toolCtx := core.NewToolContext(reqCtx, func(o *core.ToolContextOptions) { o.FunctionCallID = core.String(fc.ID) })

	tool, ok := toolRegistry[fc.Name]
	if !ok {
		return nil, fmt.Errorf("%w: tool=%s", core.ErrToolNotFound, fc.Name)
	}

	argMap, argErr := parseArgs(fc.Arguments)
	if argErr != nil {
		return nil, argErr
	}

	log.Info("agent.function.start")

	result, err := e.executeWithPlugins(ctx, reqCtx, toolCtx, tool, argMap)
	if err != nil {
		return nil, err
	}

	ev := core.NewFunctionResponseEvent(reqCtx.RunID(), reqCtx.AgentName(), fc.ID, fc.Name, result)
	ev.ApplyActions(toolCtx.EventActions())

	return ev, nil
}

// executeWithPlugins runs plugin hooks around a tool call.
func (e *parallelToolExecutor) executeWithPlugins(
	ctx context.Context,
	reqCtx core.RequestContext,
	toolCtx core.ToolContext,
	tool core.Tool,
	argMap map[string]any,
) (any, error) {
	log := logging.FromContext(ctx)

	overridden, err := reqCtx.RunBeforeTool(ctx, tool, toolCtx, argMap)
	if err != nil {
		log.Error("agent.function.before_tool.error", "error", err)
		return nil, err
	}

	var final any

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
			if herr != nil {
				log.Error("agent.function.error", "error", herr)
				return nil, herr
			}

			if recovered == nil {
				return nil, callErr
			}

			final = recovered
		} else {
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

// parseArgs parses JSON arguments string safely.
func parseArgs(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}

	var argMap map[string]any
	if err := json.Unmarshal([]byte(raw), &argMap); err != nil {
		return nil, fmt.Errorf("%w: %v", core.ErrInvalidToolArgs, err)
	}

	if argMap == nil {
		argMap = map[string]any{}
	}

	return argMap, nil
}

// panicError adapts panic to error.
func panicError(r any) error { return &panicErr{val: r, stack: debug.Stack()} }

// panicErr represents a recovered panic.
type panicErr struct {
	val   any
	stack []byte
}

func (p *panicErr) Error() string { return "panic recovered" }
