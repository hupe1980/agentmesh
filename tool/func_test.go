package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionTool_Accessors(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "string"}},
	}

	tool := NewFuncTool(
		"echo",
		"Echo input",
		schema,
		func(_ context.Context, _ core.ToolContext, args map[string]any) (any, error) { return args, nil },
	)

	assert.Equal(t, "echo", tool.Name())
	assert.Equal(t, "Echo input", tool.Description())
	assert.Equal(t, schema, tool.Parameters())
}

func TestFunctionTool_ValidationError(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
		},
		// use []any to mirror JSON decoded schemas
		"required": []any{"a"},
	}

	tool := NewFuncTool(
		"sum",
		"sum numbers",
		params,
		func(_ context.Context, _ core.ToolContext, _ map[string]any) (any, error) { return 0, nil },
	)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})

	// Missing required field
	_, err := tool.Call(context.Background(), tc, "{}")
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
	assert.Contains(t, terr.Error(), "invalid value: validating root: required: missing properties: [\"a\"]")

	// Wrong type
	_, err = tool.Call(context.Background(), tc, testutil.MustJSON(t, map[string]any{"a": "not-a-number"}))
	require.Error(t, err)
	terr, ok = err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
}

func TestFunctionTool_CanceledContext(t *testing.T) {
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	tool := NewFuncTool(
		"noop",
		"no op",
		params,
		func(_ context.Context, _ core.ToolContext, _ map[string]any) (any, error) { return nil, nil },
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})
	_, err := tool.Call(ctx, tc, "{}")
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "EXECUTION_ERROR", terr.Code)
	assert.Contains(t, terr.Error(), "canceled")
}

func TestFunctionTool_WrapsExecutionError(t *testing.T) {
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	tool := NewFuncTool(
		"fail",
		"always fails",
		params,
		func(_ context.Context, _ core.ToolContext, _ map[string]any) (any, error) {
			return nil, errors.New("boom")
		},
	)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})
	_, err := tool.Call(context.Background(), tc, "")
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
	assert.Contains(t, terr.Error(), "tool error [VALIDATION_ERROR] in fail: invalid value: empty string")
}

func TestFunctionTool_PassthroughToolError(t *testing.T) {
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	tool := NewFuncTool(
		"fail_custom",
		"fails with custom code",
		params,
		func(_ context.Context, _ core.ToolContext, _ map[string]any) (any, error) {
			return nil, NewError("fail_custom", "bad", "CUSTOM")
		},
	)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})
	_, err := tool.Call(context.Background(), tc, "{}")
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "CUSTOM", terr.Code)
	assert.Contains(t, terr.Error(), "bad")
}

func TestFunctionToolFromStruct_SchemaAndCall(t *testing.T) {
	type SumArgs struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}

	tool, err := NewFuncToolFromType(
		"sum_struct",
		"sum using struct schema",
		func(_ context.Context, _ core.ToolContext, args *SumArgs) (any, error) {
			return args.A + args.B, nil
		},
	)
	require.NoError(t, err)

	// Access schema and ensure it has expected shape
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})

	res, err := tool.Call(context.Background(), tc, testutil.MustJSON(t, map[string]any{"a": 1.5, "b": 2.5}))
	require.NoError(t, err)
	assert.Equal(t, 4.0, res)
}

// Test helper used by FuncTool tests
func dummyRequestContext() core.RequestContext {
	sessSvc := &testutil.SessionStoreMock{
		GetOrCreateFunc: func(_ context.Context, appName, userID, id string) (*core.Session, error) {
			return core.NewSession(appName, userID, id), nil
		},
		AppendEventFunc: func(ctx context.Context, sess *core.Session, ev *core.Event) error { return nil },
	}
	artSvc := &testutil.ArtifactStoreMock{
		SaveFunc:     func(ctx context.Context, _, _, _, _ string, _ core.Part) error { return nil },
		LoadFunc:     func(ctx context.Context, _, _, _, _ string) (core.Part, error) { return nil, nil },
		ListKeysFunc: func(ctx context.Context, _, _, _ string) ([]string, error) { return []string{}, nil },
		DeleteFunc:   func(ctx context.Context, _, _, _, _ string) error { return nil },
	}
	memSvc := &testutil.MemoryStoreMock{
		SearchFunc: func(ctx context.Context, _, _ string, _ string) (*core.SearchResult, error) {
			return &core.SearchResult{Memories: nil}, nil
		},
		AddSessionFunc: func(ctx context.Context, _ *core.Session) error { return nil },
	}

	appName := "app1"
	userID := "user1"
	sessionID := "sess1"
	if _, err := sessSvc.GetOrCreate(context.Background(), appName, userID, sessionID); err != nil {
		panic(err)
	}

	ag := testutil.NewMockAgent("Agent")
	return testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = ag
		p.RunID = "run1"
		p.Session = core.NewSession(appName, userID, sessionID)
		p.SessionStore = sessSvc
		p.ArtifactStore = artSvc
		p.MemoryStore = memSvc
		p.MaxModelCalls = 100
	})
}

func TestFunctionTool_ProcessModelRequest_NoOp(t *testing.T) {
	tool := NewFuncTool(
		"noop",
		"no op",
		map[string]any{"type": "object", "properties": map[string]any{}},
		func(_ context.Context, _ core.ToolContext, _ map[string]any) (any, error) { return nil, nil },
	)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})
	err := tool.ProcessModelRequest(context.Background(), tc, &core.ModelRequest{})
	assert.NoError(t, err)
}
