package tool

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------- Schema & Validation Tests --------------------

type sampleSchema struct {
	A string `json:"a" description:"Field A"`
	B *int   `json:"b" description:"Optional pointer field"`
	C int    `json:"c,omitempty" description:"Omit empty field"`
}

func TestCreateSchema(t *testing.T) {
	schema := util.CreateSchema(sampleSchema{})
	props, ok := schema["properties"].(map[string]any)
	assert.True(t, ok)
	// Properties present
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")
	assert.Contains(t, props, "c")
	// Required only includes non-pointer, non-omitempty exported fields
	req, _ := schema["required"].([]string)
	if req == nil { // reflection may produce []any
		ifaceReq, _ := schema["required"].([]any)
		for _, v := range ifaceReq {
			req = append(req, v.(string))
		}
	}
	assert.ElementsMatch(t, []string{"a"}, req)
}

func TestValidateParameters(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
		// Use []any to mirror possible JSON decoded schema shape
		"required": []any{"x"},
	}

	// Success
	err := util.ValidateParameters(map[string]any{"x": 5}, schema)
	assert.NoError(t, err)

	// Wrong type
	err = util.ValidateParameters(map[string]any{"x": "not-int"}, schema)
	assert.Error(t, err)
	require.IsType(t, &util.ValidationError{}, err)
	vErr := err.(*util.ValidationError)
	assert.Contains(t, vErr.Message, "expected type integer")
}

// -------------------- FuncTool Tests --------------------

func TestFunctionTool_Success(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
			"b": map[string]any{"type": "number"},
		},
		"required": []string{"a", "b"},
	}

	sumTool := NewFuncTool(
		"sum",
		"Add numbers",
		params,
		func(_ context.Context, _ core.ToolContext, args map[string]any) (any, error) {
			a := args["a"].(float64)
			b := args["b"].(float64)
			return a + b, nil
		},
	)

	reqCtx := dummyRequestContext()
	tc := core.NewToolContext(reqCtx, func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc1")
	})
	result, err := sumTool.Call(context.Background(), tc, map[string]any{"a": 2.0, "b": 3.0})
	assert.NoError(t, err)
	assert.Equal(t, 5.0, result)
}

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

	return core.NewRequestContext(core.RequestContextParams{
		RunID:         "run1",
		Agent:         core.AgentInfo{Name: "Agent", Type: "test"},
		UserParts:     nil,
		MaxModelCalls: 100,
		Session:       core.NewSession(appName, userID, sessionID),
		SessionStore:  sessSvc,
		ArtifactStore: artSvc,
		MemoryStore:   memSvc,
	})
}

func TestErrorFormatting(t *testing.T) {
	err := NewError("demo", "something failed", "E123")
	assert.Contains(t, err.Error(), "E123")
	assert.Contains(t, err.Error(), "demo")
}
