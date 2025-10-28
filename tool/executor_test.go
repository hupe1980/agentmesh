package tool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sleepTool simulates latency before returning its name.
type sleepTool struct {
	name  string
	delay time.Duration
}

func (t *sleepTool) Name() string               { return t.name }
func (t *sleepTool) Description() string        { return "sleep tool for testing" }
func (t *sleepTool) Parameters() map[string]any { return map[string]any{} }
func (t *sleepTool) IsLongRunning() bool        { return true }
func (t *sleepTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *sleepTool) Call(ctx context.Context, toolCtx core.ToolContext, _ string) (any, error) {
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
func (t *actionsTool) IsLongRunning() bool        { return false }
func (t *actionsTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *actionsTool) Call(ctx context.Context, tc core.ToolContext, _ string) (any, error) {
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
func (t *panicTool) IsLongRunning() bool        { return false }
func (t *panicTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *panicTool) Call(context.Context, core.ToolContext, string) (any, error) {
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
func (t *counterTool) IsLongRunning() bool        { return true }
func (t *counterTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *counterTool) Call(ctx context.Context, _ core.ToolContext, _ string) (any, error) {
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
	select {
	case <-ctx.Done():
		_ = atomic.AddInt32(t.current, -1)
		return nil, ctx.Err()
	case <-time.After(t.delay):
		_ = atomic.AddInt32(t.current, -1)
		return t.name, nil
	}
}

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
	cbCtx core.CallbackContext,
	req *core.ModelRequest,
) (*core.ModelResponse, error) {
	return nil, nil
}
func (p *toolHookPlugin) AfterModel(
	ctx context.Context,
	cbCtx core.CallbackContext,
	resp *core.ModelResponse,
) (*core.ModelResponse, error) {
	return nil, nil
}
func (p *toolHookPlugin) OnModelError(
	ctx context.Context,
	cbCtx core.CallbackContext,
	req *core.ModelRequest,
	err error,
) (*core.ModelResponse, error) {
	return nil, nil
}
func (p *toolHookPlugin) BeforeTool(
	ctx context.Context,
	tool core.Tool,
	toolCtx core.ToolContext,
	toolArgs string,
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
	toolArgs string,
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
	toolArgs string,
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

type valueTool struct{ name, value string }

func (t *valueTool) Name() string               { return t.name }
func (t *valueTool) Description() string        { return "value tool" }
func (t *valueTool) Parameters() map[string]any { return map[string]any{} }
func (t *valueTool) IsLongRunning() bool        { return false }
func (t *valueTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *valueTool) Call(context.Context, core.ToolContext, string) (any, error) {
	return t.value, nil
}

// --- Tests ---

func TestParallelToolExecutor_OutOfOrder(t *testing.T) {
	reg := map[string]core.Tool{
		"slow": &sleepTool{name: "slow", delay: 120 * time.Millisecond},
		"fast": &sleepTool{name: "fast", delay: 10 * time.Millisecond},
	}
	calls := []*core.FunctionCall{{ID: "1", Name: "slow"}, {ID: "2", Name: "fast"}}
	exec := NewParallelToolExecutor(2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := exec.Execute(ctx, testutil.NewTestRequestContext(), reg, calls)
	require.NoError(t, err)
	require.Len(t, events, 2)
	got := []string{events[0].GetFunctionResponses()[0].Name, events[1].GetFunctionResponses()[0].Name}
	assert.Equal(t, []string{"fast", "slow"}, got)
}

func TestParallelToolExecutor_ActionsApplied(t *testing.T) {
	reg := map[string]core.Tool{"actions": &actionsTool{name: "actions"}}
	calls := []*core.FunctionCall{{ID: "a1", Name: "actions"}}
	exec := NewParallelToolExecutor(1)
	events, err := exec.Execute(context.Background(), testutil.NewTestRequestContext(), reg, calls)
	require.NoError(t, err)
	require.Len(t, events, 1)
	Ev := events[0]
	assert.Equal(t, map[string]any{"k": "v"}, Ev.Actions.StateDelta.Or(nil))
	assert.Equal(t, map[string]int{"a": 1}, Ev.Actions.ArtifactDelta.Or(nil))
	assert.Equal(t, "ChildAgent", Ev.Actions.TransferToAgent.Or(""))
	assert.True(t, Ev.Actions.Escalate.Or(false))
	assert.True(t, Ev.Actions.SkipSummarization.Or(false))
}

func TestParallelToolExecutor_PanicRecovery(t *testing.T) {
	reg := map[string]core.Tool{"panic": &panicTool{name: "panic"}}
	calls := []*core.FunctionCall{{ID: "p1", Name: "panic"}}
	exec := NewParallelToolExecutor(2)
	_, err := exec.Execute(context.Background(), testutil.NewTestRequestContext(), reg, calls)
	assert.Error(t, err)
}

func TestParallelToolExecutor_ToolNotFound(t *testing.T) {
	reg := map[string]core.Tool{}
	calls := []*core.FunctionCall{{ID: "m1", Name: "missing"}}
	exec := NewParallelToolExecutor(4)
	_, err := exec.Execute(context.Background(), testutil.NewTestRequestContext(), reg, calls)
	assert.Error(t, err)
}

func TestParallelToolExecutor_PluginShortCircuit(t *testing.T) {
	plug := &toolHookPlugin{beforeResult: "precomputed"}
	reg := map[string]core.Tool{"v": &valueTool{name: "v", value: "ignored"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "v"}}
	exec := NewParallelToolExecutor(1)
	rc := testutil.NewTestRequestContext(
		func(rcp *core.RequestContextParams) {
			rcp.PluginManager = core.NewPluginManager(plug)
		},
	)
	events, err := exec.Execute(context.Background(), rc, reg, calls)
	require.NoError(t, err)
	require.Len(t, events, 1)
	fr := events[0].GetFunctionResponses()
	require.Len(t, fr, 1)
	assert.Equal(t, "precomputed", fr[0].Response)
	assert.Equal(t, 1, plug.beforeCalled)
	assert.Equal(t, 1, plug.afterCalled)
	assert.Equal(t, 0, plug.errorCalled)
}

func TestParallelToolExecutor_PluginRecovery(t *testing.T) {
	plug := &toolHookPlugin{errorResult: "recovered"}
	reg := map[string]core.Tool{"panic": &panicTool{name: "panic"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "panic"}}
	exec := NewParallelToolExecutor(1)
	rc := testutil.NewTestRequestContext(
		func(rcp *core.RequestContextParams) {
			rcp.PluginManager = core.NewPluginManager(plug)
		},
	)
	events, err := exec.Execute(context.Background(), rc, reg, calls)
	require.NoError(t, err)
	require.Len(t, events, 1)
	fr := events[0].GetFunctionResponses()
	require.Len(t, fr, 1)
	assert.Equal(t, "recovered", fr[0].Response)
	assert.Equal(t, 1, plug.errorCalled)
	assert.Equal(t, 1, plug.afterCalled)
}

func TestParallelToolExecutor_PluginBeforeFailure(t *testing.T) {
	plug := &toolHookPlugin{failBefore: errors.New("before fail")}
	reg := map[string]core.Tool{"v": &valueTool{name: "v", value: "x"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "v"}}
	exec := NewParallelToolExecutor(1)
	rc := testutil.NewTestRequestContext(
		func(rcp *core.RequestContextParams) {
			rcp.PluginManager = core.NewPluginManager(plug)
		},
	)
	_, err := exec.Execute(context.Background(), rc, reg, calls)
	assert.Error(t, err)
	assert.Equal(t, 1, plug.beforeCalled)
	assert.Equal(t, 0, plug.afterCalled)
	assert.Equal(t, 0, plug.errorCalled)
}

func TestParallelToolExecutor_PluginAfterFailure(t *testing.T) {
	plug := &toolHookPlugin{afterResult: "unused", failAfter: errors.New("after fail")}
	reg := map[string]core.Tool{"v": &valueTool{name: "v", value: "orig"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "v"}}
	exec := NewParallelToolExecutor(1)
	rc := testutil.NewTestRequestContext(
		func(rcp *core.RequestContextParams) {
			rcp.PluginManager = core.NewPluginManager(plug)
		},
	)
	_, err := exec.Execute(context.Background(), rc, reg, calls)
	assert.Error(t, err)
	assert.Equal(t, 1, plug.beforeCalled)
	assert.Equal(t, 1, plug.afterCalled)
	assert.Equal(t, 0, plug.errorCalled)
}

func TestParallelToolExecutor_PluginErrorHookFailure(t *testing.T) {
	plug := &toolHookPlugin{failError: errors.New("error hook fail")}
	reg := map[string]core.Tool{"panic": &panicTool{name: "panic"}}
	calls := []*core.FunctionCall{{ID: "1", Name: "panic"}}
	exec := NewParallelToolExecutor(1)
	rc := testutil.NewTestRequestContext(
		func(rcp *core.RequestContextParams) {
			rcp.PluginManager = core.NewPluginManager(plug)
		},
	)
	_, err := exec.Execute(context.Background(), rc, reg, calls)
	assert.Error(t, err)
	assert.Equal(t, 1, plug.errorCalled)
}

func TestParallelToolExecutor_RespectsMaxParallel(t *testing.T) {
	var current, max int32
	reg := map[string]core.Tool{"c": &counterTool{name: "c", delay: 40 * time.Millisecond, current: &current, max: &max}}
	calls := []*core.FunctionCall{{ID: "1", Name: "c"}, {ID: "2", Name: "c"}, {ID: "3", Name: "c"}}
	exec := NewParallelToolExecutor(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := exec.Execute(ctx, testutil.NewTestRequestContext(), reg, calls)
	require.NoError(t, err)
	assert.LessOrEqual(t, int(max), 1)
}

type failingTool struct {
	name string
	err  error
}

func (t *failingTool) Name() string               { return t.name }
func (t *failingTool) Description() string        { return "failing tool" }
func (t *failingTool) Parameters() map[string]any { return map[string]any{} }
func (t *failingTool) IsLongRunning() bool        { return false }
func (t *failingTool) ProcessModelRequest(context.Context, core.ToolContext, *core.ModelRequest) error {
	return nil
}
func (t *failingTool) Call(context.Context, core.ToolContext, string) (any, error) {
	return nil, t.err
}

func TestParallelToolExecutor_AggregatesMultipleErrors(t *testing.T) {
	errA := errors.New("fail A")
	errB := errors.New("fail B")
	reg := map[string]core.Tool{
		"failA": &failingTool{name: "failA", err: errA},
		"failB": &failingTool{name: "failB", err: errB},
		"ok":    &valueTool{name: "ok", value: "success"},
	}
	calls := []*core.FunctionCall{{ID: "1", Name: "failA"}, {ID: "2", Name: "failB"}, {ID: "3", Name: "ok"}}
	exec := NewParallelToolExecutor(3)
	events, err := exec.Execute(context.Background(), testutil.NewTestRequestContext(), reg, calls)
	require.Error(t, err)
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)
	require.Len(t, events, 1)
	fr := events[0].GetFunctionResponses()
	require.Len(t, fr, 1)
	assert.Equal(t, "success", fr[0].Response)
}
