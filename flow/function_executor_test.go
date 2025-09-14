package flow

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test tool implementations
type sleepTool struct {
	name  string
	delay time.Duration
}

func (t *sleepTool) Name() string               { return t.name }
func (t *sleepTool) Description() string        { return "sleep tool for testing" }
func (t *sleepTool) Parameters() map[string]any { return map[string]any{} }
func (t *sleepTool) ProcessModelRequest(ctx context.Context, toolCtx core.ToolContext, _ *core.ModelRequest) error {
	return nil
}
func (t *sleepTool) Call(ctx context.Context, toolCtx core.ToolContext, toolArgs map[string]any) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(t.delay):
		return t.name, nil
	}
}

type actionsTool struct{ name string }

func (t *actionsTool) Name() string               { return t.name }
func (t *actionsTool) Description() string        { return "actions tool" }
func (t *actionsTool) Parameters() map[string]any { return map[string]any{} }
func (t *actionsTool) ProcessModelRequest(ctx context.Context, toolCtx core.ToolContext, _ *core.ModelRequest) error {
	return nil
}
func (t *actionsTool) Call(ctx context.Context, tc core.ToolContext, toolArgs map[string]any) (any, error) {
	// mutate actions through accessor
	acts := tc.EventActions()
	acts.StateDelta = core.Map(map[string]any{"k": "v"})
	acts.ArtifactDelta = core.Map(map[string]int{"a": 1})
	tc.TransferToAgent("ChildAgent")
	tc.Escalate()
	tc.SkipSummarization()

	return "ok", nil
}

type panicTool struct{ name string }

func (t *panicTool) Name() string               { return t.name }
func (t *panicTool) Description() string        { return "panic tool" }
func (t *panicTool) Parameters() map[string]any { return map[string]any{} }
func (t *panicTool) ProcessModelRequest(ctx context.Context, toolCtx core.ToolContext, _ *core.ModelRequest) error {
	return nil
}
func (t *panicTool) Call(context.Context, core.ToolContext, map[string]any) (any, error) {
	panic("boom")
}

type counterTool struct {
	name    string
	delay   time.Duration
	current *int32
	max     *int32
}

func (t *counterTool) Name() string               { return t.name }
func (t *counterTool) Description() string        { return "counter tool" }
func (t *counterTool) Parameters() map[string]any { return map[string]any{} }
func (t *counterTool) ProcessModelRequest(ctx context.Context, toolCtx core.ToolContext, _ *core.ModelRequest) error {
	return nil
}
func (t *counterTool) Call(ctx context.Context, toolCtx core.ToolContext, toolArgs map[string]any) (any, error) {
	c := atomic.AddInt32(t.current, 1)
	for {
		m := atomic.LoadInt32(t.max)
		if c > m {
			if atomic.CompareAndSwapInt32(t.max, m, c) {
				break
			}
			continue
		}
		break
	}
	// simulate work
	select {
	case <-ctx.Done():
		_ = atomic.AddInt32(t.current, -1)
		return nil, ctx.Err()
	case <-time.After(t.delay):
		_ = atomic.AddInt32(t.current, -1)
		return t.name, nil
	}
}

func TestFunctionExecutor_EmitsOutOfOrder(t *testing.T) {
	t.Parallel()

	// Prepare registry with slow and fast tools
	reg := map[string]core.Tool{
		"slow": &sleepTool{name: "slow", delay: 120 * time.Millisecond},
		"fast": &sleepTool{name: "fast", delay: 10 * time.Millisecond},
	}

	// Calls provided in order: slow, fast
	calls := []*core.FunctionCall{
		{ID: "1", Name: "slow"},
		{ID: "2", Name: "fast"},
	}

	// Collect emitted order
	var mu sync.Mutex
	var got []string
	emit := func(ev *core.Event) error {
		mu.Lock()
		defer mu.Unlock()
		frs := ev.GetFunctionResponses()
		require.NotEmpty(t, frs)
		got = append(got, frs[0].Name)
		return nil
	}

	exec := NewParallelFunctionExecutor(2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := exec.Execute(ctx, testutil.NewTestRequestContext(), reg, calls, emit)
	require.NoError(t, err)

	// Expect fast emitted before slow (out-of-order). This should be stable given delays.
	require.Len(t, got, 2)
	assert.Equal(t, []string{"fast", "slow"}, got)
}

func TestFunctionExecutor_AppliesToolActions(t *testing.T) {
	reg := map[string]core.Tool{"actions": &actionsTool{name: "actions"}}
	calls := []*core.FunctionCall{{ID: "a1", Name: "actions"}}
	var evs []*core.Event
	var mu sync.Mutex
	emit := func(ev *core.Event) error { mu.Lock(); evs = append(evs, ev); mu.Unlock(); return nil }

	exec := NewParallelFunctionExecutor(1)
	err := exec.Execute(context.Background(), testutil.NewTestRequestContext(), reg, calls, emit)
	require.NoError(t, err)
	require.Len(t, evs, 1)

	ev := evs[0]

	// State/Artifact deltas merged
	assert.Equal(t, map[string]any{"k": "v"}, ev.Actions.StateDelta.Or(nil))
	assert.Equal(t, map[string]int{"a": 1}, ev.Actions.ArtifactDelta.Or(nil))
	// Transfer / escalate / skip summarization flags present
	require.NotNil(t, ev.Actions.TransferToAgent)
	assert.Equal(t, "ChildAgent", ev.Actions.TransferToAgent.Or(""))
	require.NotNil(t, ev.Actions.Escalate)
	assert.True(t, ev.Actions.Escalate.Or(false))
	require.NotNil(t, ev.Actions.SkipSummarization)
	assert.True(t, ev.Actions.SkipSummarization.Or(false))
}

func TestFunctionExecutor_RecoversFromPanic(t *testing.T) {
	reg := map[string]core.Tool{"panic": &panicTool{name: "panic"}}
	calls := []*core.FunctionCall{{ID: "p1", Name: "panic"}}
	exec := NewParallelFunctionExecutor(2)
	err := exec.Execute(
		context.Background(),
		testutil.NewTestRequestContext(),
		reg,
		calls,
		func(*core.Event) error { return nil },
	)
	assert.Error(t, err)
}

func TestFunctionExecutor_ToolNotFound(t *testing.T) {
	reg := map[string]core.Tool{}
	calls := []*core.FunctionCall{{ID: "m1", Name: "missing"}}
	exec := NewParallelFunctionExecutor(4)
	err := exec.Execute(
		context.Background(),
		testutil.NewTestRequestContext(),
		reg,
		calls,
		func(*core.Event) error { return nil },
	)
	assert.Error(t, err)
}

func TestFunctionExecutor_InvalidArgs(t *testing.T) {
	reg := map[string]core.Tool{"n": &sleepTool{name: "n", delay: 1 * time.Millisecond}}
	calls := []*core.FunctionCall{{ID: "x", Name: "n", Arguments: "{"}}
	exec := NewParallelFunctionExecutor(1)
	err := exec.Execute(
		context.Background(),
		testutil.NewTestRequestContext(),
		reg,
		calls,
		func(*core.Event) error { return nil },
	)
	assert.Error(t, err)
}

// plugin capturing hook calls and allowing overrides
type toolHookPlugin struct {
	beforeCalled int
	afterCalled  int
	errorCalled  int
	beforeResult any
	afterResult  any
	errorResult  any
	failBefore   error
	failAfter    error
	failError    error
}

func (p *toolHookPlugin) OnUserParts(context.Context, core.RequestContext, []core.Part) ([]core.Part, error) {
	return nil, nil
}
func (p *toolHookPlugin) BeforeRun(context.Context, core.RequestContext) ([]core.Part, error) {
	return nil, nil
}
func (p *toolHookPlugin) AfterRun(context.Context, core.RequestContext) error { return nil }
func (p *toolHookPlugin) OnEvent(context.Context, core.RequestContext, *core.Event) (*core.Event, error) {
	return nil, nil
}
func (p *toolHookPlugin) BeforeAgent(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error) {
	return nil, nil
}
func (p *toolHookPlugin) AfterAgent(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error) {
	return nil, nil
}
func (p *toolHookPlugin) BeforeModel(
	ctx context.Context,
	cb core.CallbackContext,
	req *core.ModelRequest,
) (*core.ModelResponse, error) {
	return nil, nil
}
func (p *toolHookPlugin) AfterModel(
	ctx context.Context,
	cb core.CallbackContext,
	resp *core.ModelResponse,
) (*core.ModelResponse, error) {
	return nil, nil
}
func (p *toolHookPlugin) OnModelError(
	ctx context.Context,
	cb core.CallbackContext,
	req *core.ModelRequest,
	err error,
) (*core.ModelResponse, error) {
	return nil, nil
}
func (p *toolHookPlugin) BeforeTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs map[string]any,
) (any, error) {
	p.beforeCalled++
	if p.failBefore != nil {
		return nil, p.failBefore
	}
	return p.beforeResult, nil
}
func (p *toolHookPlugin) AfterTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs map[string]any,
	result any,
) (any, error) {
	p.afterCalled++
	if p.failAfter != nil {
		return nil, p.failAfter
	}
	if p.afterResult != nil {
		return p.afterResult, nil
	}
	return nil, nil
}
func (p *toolHookPlugin) OnToolError(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs map[string]any,
	err error,
) (any, error) {
	p.errorCalled++
	if p.failError != nil {
		return nil, p.failError
	}
	if p.errorResult != nil {
		return p.errorResult, nil
	}
	return nil, nil
}

// simple tool returning static value
type valueTool struct{ name, value string }

func (t *valueTool) Name() string               { return t.name }
func (t *valueTool) Description() string        { return "value tool" }
func (t *valueTool) Parameters() map[string]any { return map[string]any{} }
func (t *valueTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *valueTool) Call(context.Context, core.ToolContext, map[string]any) (any, error) {
	return t.value, nil
}

func TestFunctionExecutor_Plugin_BeforeShortCircuit(t *testing.T) {
	plug := &toolHookPlugin{beforeResult: "precomputed"}
	reg := map[string]core.Tool{"v": &valueTool{name: "v", value: "ignored"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "v"}}
	exec := NewParallelFunctionExecutor(1)

	rc := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.PluginManager = core.NewPluginManager(plug)
	})

	var out []*core.Event
	err := exec.Execute(
		context.Background(),
		rc,
		reg,
		calls,
		func(ev *core.Event) error {
			out = append(out, ev)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, out, 1)
	fr := out[0].GetFunctionResponses()
	require.Len(t, fr, 1)
	assert.Equal(t, "precomputed", fr[0].Response)
	assert.Equal(t, 1, plug.beforeCalled)
	assert.Equal(t, 1, plug.afterCalled) // after should run on short-circuit
	assert.Equal(t, 0, plug.errorCalled)
}

func TestFunctionExecutor_Plugin_ErrorRecovery(t *testing.T) {
	plug := &toolHookPlugin{errorResult: "recovered"}
	failing := &panicTool{name: "panic"}
	reg := map[string]core.Tool{"panic": failing}
	calls := []*core.FunctionCall{{ID: "1", Name: "panic"}}
	exec := NewParallelFunctionExecutor(1)
	rc := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.PluginManager = core.NewPluginManager(plug)
	})

	var out []*core.Event
	err := exec.Execute(
		context.Background(),
		rc,
		reg,
		calls,
		func(ev *core.Event) error {
			out = append(out, ev)
			return nil
		},
	)
	// Original panic -> error -> recovered by plugin => no error returned
	require.NoError(t, err)
	require.Len(t, out, 1)
	fr := out[0].GetFunctionResponses()
	require.Len(t, fr, 1)
	assert.Equal(t, "recovered", fr[0].Response)
	assert.Equal(t, 1, plug.errorCalled)
	assert.Equal(t, 1, plug.afterCalled) // after runs after recovery
}

func TestFunctionExecutor_Plugin_BeforeFailureStops(t *testing.T) {
	plug := &toolHookPlugin{failBefore: errors.New("before fail")}
	reg := map[string]core.Tool{"v": &valueTool{name: "v", value: "x"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "v"}}
	exec := NewParallelFunctionExecutor(1)
	rc := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.PluginManager = core.NewPluginManager(plug)
	})

	err := exec.Execute(context.Background(), rc, reg, calls, func(*core.Event) error { return nil })
	assert.Error(t, err)
	assert.Equal(t, 1, plug.beforeCalled)
	assert.Equal(t, 0, plug.afterCalled)
	assert.Equal(t, 0, plug.errorCalled)
}

func TestFunctionExecutor_Plugin_AfterFailureStops(t *testing.T) {
	plug := &toolHookPlugin{afterResult: "unused", failAfter: errors.New("after fail")}
	reg := map[string]core.Tool{"v": &valueTool{name: "v", value: "orig"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "v"}}
	exec := NewParallelFunctionExecutor(1)
	rc := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.PluginManager = core.NewPluginManager(plug)
	})

	err := exec.Execute(context.Background(), rc, reg, calls, func(*core.Event) error { return nil })
	assert.Error(t, err)
	assert.Equal(t, 1, plug.beforeCalled)
	assert.Equal(t, 1, plug.afterCalled)
	assert.Equal(t, 0, plug.errorCalled)
}

func TestFunctionExecutor_Plugin_ErrorHookFailure(t *testing.T) {
	plug := &toolHookPlugin{failError: errors.New("error hook fail")}
	reg := map[string]core.Tool{"panic": &panicTool{name: "panic"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "panic"}}
	exec := NewParallelFunctionExecutor(1)
	rc := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.PluginManager = core.NewPluginManager(plug)
	})

	err := exec.Execute(context.Background(), rc, reg, calls, func(*core.Event) error { return nil })
	assert.Error(t, err)
	assert.Equal(t, 1, plug.errorCalled)
}

func TestFunctionExecutor_RespectsMaxParallel(t *testing.T) {
	var current, max int32
	reg := map[string]core.Tool{
		"c": &counterTool{name: "c", delay: 40 * time.Millisecond, current: &current, max: &max},
	}

	// Three calls to the same tool
	calls := []*core.FunctionCall{{ID: "1", Name: "c"}, {ID: "2", Name: "c"}, {ID: "3", Name: "c"}}
	emit := func(*core.Event) error { return nil }

	exec := NewParallelFunctionExecutor(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := exec.Execute(ctx, testutil.NewTestRequestContext(), reg, calls, emit)
	require.NoError(t, err)

	// With MaxParallel=1 we should never observe >1 concurrent execution
	assert.LessOrEqual(t, int(max), 1)
}

// failing tool used to generate distinct errors
type failingTool struct {
	name string
	err  error
}

func (t *failingTool) Name() string               { return t.name }
func (t *failingTool) Description() string        { return "failing tool" }
func (t *failingTool) Parameters() map[string]any { return map[string]any{} }
func (t *failingTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *failingTool) Call(context.Context, core.ToolContext, map[string]any) (any, error) {
	return nil, t.err
}

func TestFunctionExecutor_AggregatesMultipleErrors(t *testing.T) {
	t.Parallel()

	errA := errors.New("fail A")
	errB := errors.New("fail B")

	reg := map[string]core.Tool{
		"failA": &failingTool{name: "failA", err: errA},
		"failB": &failingTool{name: "failB", err: errB},
		"ok":    &valueTool{name: "ok", value: "success"},
	}

	calls := []*core.FunctionCall{{ID: "1", Name: "failA"}, {ID: "2", Name: "failB"}, {ID: "3", Name: "ok"}}

	var mu sync.Mutex
	var events []*core.Event
	emit := func(ev *core.Event) error { mu.Lock(); events = append(events, ev); mu.Unlock(); return nil }

	exec := NewParallelFunctionExecutor(3)

	err := exec.Execute(context.Background(), testutil.NewTestRequestContext(), reg, calls, emit)
	require.Error(t, err)
	// Aggregated error should contain both underlying errors
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)

	// Only the successful tool should have emitted an event
	require.Len(t, events, 1)
	fr := events[0].GetFunctionResponses()
	require.Len(t, fr, 1)
	assert.Equal(t, "success", fr[0].Response)
}
