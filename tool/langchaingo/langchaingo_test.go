package langchaingo

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dummyLangChainTool is a minimal implementation of the langchaingo Tool interface
// used for testing the adapter.
type dummyLangChainTool struct {
	name        string
	description string
	result      any
	err         error
	calls       int
}

func (d *dummyLangChainTool) Name() string        { return d.name }
func (d *dummyLangChainTool) Description() string { return d.description }
func (d *dummyLangChainTool) Call(ctx context.Context, input string) (string, error) {
	// Record the call and echo / error
	d.calls++
	if d.err != nil {
		return "", d.err
	}
	// Support returning non-string via fmt sprint path; adapter expects any -> marshalled later.
	if s, ok := d.result.(string); ok {
		return s + ":" + input, nil
	}
	return "ok:" + input, nil
}

func TestLangChain_New_Defaults(t *testing.T) {
	d := &dummyLangChainTool{name: "lc_echo", description: "echoes input"}
	wrapped := New(d)

	assert.Equal(t, d.name, wrapped.Name())
	assert.Equal(t, d.description, wrapped.Description())
	params := wrapped.Parameters()
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	_, exists := props["__arg1"]
	assert.True(t, exists, "__arg1 property should exist")
}

func TestLangChain_New_OverrideOptions(t *testing.T) {
	d := &dummyLangChainTool{name: "orig", description: "original"}
	wrapped := New(d, func(o *Options) {
		o.Name = "override_name"
		o.Description = "override desc"
	})

	assert.Equal(t, "override_name", wrapped.Name())
	assert.Equal(t, "override desc", wrapped.Description())
}

func TestLangChain_Call_Success(t *testing.T) {
	d := &dummyLangChainTool{name: "lc", description: "d"}
	wrapped := New(d)
	ctx := context.Background()
	toolCtx := core.NewToolContext(
		dummyRequestContext(),
		func(o *core.ToolContextOptions) { o.FunctionCallID = core.String("fc1") },
	)

	res, err := wrapped.Call(ctx, toolCtx, map[string]any{"__arg1": "hello"})
	require.NoError(t, err)
	assert.Contains(t, res.(string), "hello")
	assert.Equal(t, 1, d.calls)
}

func TestLangChain_Call_MissingArg(t *testing.T) {
	d := &dummyLangChainTool{name: "lc", description: "d"}
	wrapped := New(d)
	ctx := context.Background()
	toolCtx := core.NewToolContext(
		dummyRequestContext(),
		func(o *core.ToolContextOptions) { o.FunctionCallID = core.String("fc1") },
	)

	_, err := wrapped.Call(ctx, toolCtx, map[string]any{})
	require.Error(t, err)
	terr, ok := err.(*tool.Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
}

func TestLangChain_Call_UnderlyingError(t *testing.T) {
	underErr := errors.New("boom")
	d := &dummyLangChainTool{name: "lc", description: "d", err: underErr}
	wrapped := New(d)
	ctx := context.Background()
	toolCtx := core.NewToolContext(
		dummyRequestContext(),
		func(o *core.ToolContextOptions) { o.FunctionCallID = core.String("fc1") },
	)

	_, err := wrapped.Call(ctx, toolCtx, map[string]any{"__arg1": "x"})
	// The adapter does not wrap errors itself; underlying func tool should wrap as EXECUTION_ERROR
	require.Error(t, err)
	terr, ok := err.(*tool.Error)
	require.True(t, ok)
	assert.Equal(t, "EXECUTION_ERROR", terr.Code)
	assert.Contains(t, terr.Error(), "boom")
}

// helper: mimic testutil request context (simplified) without importing internal packages.
func dummyRequestContext() core.RequestContext {
	sess := core.NewSession("app", "user", "sess1")
	return core.NewRequestContext(core.RequestContextParams{
		RunID:   "run1",
		Agent:   core.AgentInfo{Name: "agent", Type: "test"},
		Session: sess,
	})
}
