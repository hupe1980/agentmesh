package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTool struct {
	name   string
	desc   string
	params map[string]any
}

func (m mockTool) Name() string               { return m.name }
func (m mockTool) Description() string        { return m.desc }
func (m mockTool) Parameters() map[string]any { return m.params }
func (m mockTool) ProcessModelRequest(_ context.Context, _ ToolContext, _ *ModelRequest) error {
	return nil
}
func (m mockTool) Call(ctx context.Context, _ ToolContext, _ map[string]any) (any, error) {
	return nil, nil
}

func TestRequest_AppendInstructions(t *testing.T) {
	var r ModelRequest

	// append first fragment
	r.AppendInstructions("You are helpful.")
	assert.Equal(t, "You are helpful.", r.Instructions)

	// append multiple with empties ignored
	r.AppendInstructions("", "Be concise.", "Prefer JSON.")
	assert.Equal(t, "You are helpful.\n\nBe concise.\n\nPrefer JSON.", r.Instructions)

	// append nothing (empties only) should not change
	r.AppendInstructions("", "")
	assert.Equal(t, "You are helpful.\n\nBe concise.\n\nPrefer JSON.", r.Instructions)
}

func TestRequest_AddTool_New(t *testing.T) {
	r := &ModelRequest{}
	t1 := mockTool{
		name:   "calc",
		desc:   "calculator",
		params: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "number"}}},
	}

	r.AddTool(t1)

	// registry populated
	require.NotNil(t, r.ToolRegistry)
	assert.Contains(t, r.ToolRegistry, "calc")
	assert.Equal(t, t1.desc, r.Tools[0].Function.Description)
	assert.Equal(t, t1.name, r.Tools[0].Function.Name)
}

func TestRequest_AddTool_Replace(t *testing.T) {
	r := &ModelRequest{}
	t1 := mockTool{name: "echo", desc: "first", params: map[string]any{"type": "object"}}

	r.AddTool(t1)
	require.Len(t, r.Tools, 1)

	// add with same name, but different description/params
	t2 := mockTool{name: "echo", desc: "second", params: map[string]any{"type": "object", "required": []string{"msg"}}}
	r.AddTool(t2)

	// still one definition, replaced
	require.Len(t, r.Tools, 1)
	assert.Equal(t, "echo", r.Tools[0].Function.Name)
	assert.Equal(t, "second", r.Tools[0].Function.Description)

	// registry points to latest tool
	assert.Equal(t, t2.desc, r.ToolRegistry["echo"].Description())
}

func TestRequest_AddTools_Multiple(t *testing.T) {
	r := &ModelRequest{}
	r.AddTools(
		mockTool{name: "a", desc: "da", params: map[string]any{"type": "object"}},
		mockTool{name: "b", desc: "db", params: map[string]any{"type": "object"}},
	)

	// two definitions, order preserved
	require.Len(t, r.Tools, 2)
	assert.Equal(t, "a", r.Tools[0].Function.Name)
	assert.Equal(t, "b", r.Tools[1].Function.Name)

	// registry has both
	assert.ElementsMatch(t, []string{"a", "b"}, keys(r.ToolRegistry))
}

// helper to get map keys (string)
func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
