package flow

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/tool"
)

// FunctionExecutor executes a batch of function/tool calls possibly in parallel and emits
// function response events through the provided emit callback. Implementations must:
//   - Respect runCtx.Context cancellation
//   - Never panic (recover internally and emit error events)
//   - Emit exactly one FunctionResponse event per incoming FunctionCall
//   - Apply ToolContext accumulated actions to emitted events
//
// The emit callback is responsible for persistence synchronization (resume handling).
type FunctionExecutor interface {
	Execute(runCtx *core.RunContext, agent FlowAgent, toolRegistry map[string]tool.Tool, fnCalls []core.FunctionCall, emit func(core.Event) error)
}

// FunctionExecutorConfig configures the default parallel executor.
type FunctionExecutorConfig struct {
	MaxParallel    int  // 0 or <1 => no explicit limit (len(fnCalls))
	PreserveOrder  bool // if true, buffer results and emit in original order
	LogStartEvents bool // log a start line per function
}

// parallelFunctionExecutor is the default implementation.
type parallelFunctionExecutor struct {
	cfg FunctionExecutorConfig
}

// NewParallelFunctionExecutor constructs a new executor with the given config.
func NewParallelFunctionExecutor(cfg FunctionExecutorConfig) FunctionExecutor {
	return &parallelFunctionExecutor{cfg: cfg}
}

func (e *parallelFunctionExecutor) Execute(
	runCtx *core.RunContext,
	agent FlowAgent,
	toolRegistry map[string]tool.Tool,
	fnCalls []core.FunctionCall,
	emit func(core.Event) error,
) {
	n := len(fnCalls)
	if n == 0 {
		return
	}

	// Fast path: single call, execute inline.
	if n == 1 {
		e.executeSingle(runCtx, agent, toolRegistry, fnCalls[0], emit)
		return
	}

	maxPar := e.cfg.MaxParallel
	if maxPar <= 0 || maxPar > n {
		maxPar = n
	}

	type fnResult struct {
		index int
		event core.Event
	}

	results := make([]fnResult, n) // used only if PreserveOrder
	var mu sync.Mutex              // protects unordered emit & results writes
	var wg sync.WaitGroup

	sem := make(chan struct{}, maxPar)

	batchStart := time.Now()
	for i := range fnCalls {
		if runCtx.Context.Err() != nil { // pre-check cancellation
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, fc core.FunctionCall) {
			defer wg.Done()
			defer func() { <-sem }()

			if runCtx.Context.Err() != nil {
				return
			}

			toolCtx := core.NewToolContext(runCtx, fc.ID)
			if e.cfg.LogStartEvents {
				runCtx.LogInfo(
					"agent.function.start",
					"agent", agent.GetName(),
					"function", fc.Name,
					"function_call_id", fc.ID,
				)
			}

			execStart := time.Now()
			var (
				result any
				err    error
			)
			func() { // panic safety
				defer func() {
					if r := recover(); r != nil {
						err = panicError(r)
						runCtx.LogError("agent.function.panic", "agent", agent.GetName(), "function", fc.Name, "recover", r)
					}
				}()
				result, err = executeTool(toolRegistry, toolCtx, fc.Name, fc.Arguments)
			}()
			dur := time.Since(execStart)

			runCtx.LogInfo(
				"agent.function.executed",
				"agent", agent.GetName(),
				"function", fc.Name,
				"duration_ms", dur.Milliseconds(),
				"error", err != nil,
			)

			respEv := core.NewFunctionResponseEvent(agent.GetName(), fc.ID, fc.Name, result, err)
			toolCtx.InternalApplyActions(&respEv)

			if e.cfg.PreserveOrder {
				mu.Lock()
				results[idx] = fnResult{index: idx, event: respEv}
				mu.Unlock()
			} else {
				if err := emit(respEv); err != nil {
					runCtx.LogError("agent.function.emit.error", "function", fc.Name, "error", err.Error())
				}
			}
		}(i, fnCalls[i])
	}

	wg.Wait()

	if e.cfg.PreserveOrder {
		for i := 0; i < n; i++ {
			ev := results[i].event
			if ev.ID == "" {
				continue
			}
			if err := emit(ev); err != nil {
				runCtx.LogError("agent.function.emit.error", "function", fnCalls[i].Name, "error", err.Error())
			}
		}
	}

	runCtx.LogDebug(
		"agent.functions.batch.complete",
		"agent", agent.GetName(),
		"count", n,
		"parallelism", maxPar,
		"preserve_order", e.cfg.PreserveOrder,
		"duration_ms", time.Since(batchStart).Milliseconds(),
	)
}

func (e *parallelFunctionExecutor) executeSingle(
	runCtx *core.RunContext,
	agent FlowAgent,
	toolRegistry map[string]tool.Tool,
	fc core.FunctionCall,
	emit func(core.Event) error,
) {
	toolCtx := core.NewToolContext(runCtx, fc.ID)
	if e.cfg.LogStartEvents {
		runCtx.LogInfo("agent.function.start", "agent", agent.GetName(), "function", fc.Name, "function_call_id", fc.ID)
	}
	start := time.Now()
	var (
		result any
		err    error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = panicError(r)
				runCtx.LogError("agent.function.panic", "agent", agent.GetName(), "function", fc.Name, "recover", r)
			}
		}()
		result, err = executeTool(toolRegistry, toolCtx, fc.Name, fc.Arguments)
	}()
	dur := time.Since(start)
	runCtx.LogInfo(
		"agent.function.executed",
		"agent", agent.GetName(),
		"function", fc.Name,
		"duration_ms", dur.Milliseconds(),
		"error", err != nil,
	)
	respEv := core.NewFunctionResponseEvent(agent.GetName(), fc.ID, fc.Name, result, err)
	toolCtx.InternalApplyActions(&respEv)
	_ = emit(respEv)
}

// panicError converts a recovered panic value to an error without pulling external dependencies.
func panicError(r any) error { return &panicErr{val: r, stack: debug.Stack()} }

type panicErr struct {
	val   any
	stack []byte
}

func (p *panicErr) Error() string { return "panic recovered" }

// executeTool centralizes tool lookup & execution using agent tool registry.
func executeTool(toolRegistry map[string]tool.Tool, toolCtx *core.ToolContext, toolName, args string) (any, error) {
	impl, ok := toolRegistry[toolName]
	if !ok {
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	var argMap map[string]any
	if args == "" {
		argMap = map[string]any{}
	} else if err := json.Unmarshal([]byte(args), &argMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal args: %w", err)
	}

	return impl.Call(toolCtx, argMap)
}
