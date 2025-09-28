package tool

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferToAgentTool_SetsActionAndReturnsPayload(t *testing.T) {
	tool, err := NewTransferToAgentTool()
	require.NoError(t, err)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc-1")
	})
	res, err := tool.Call(context.Background(), tc, testutil.MustJSON(t, map[string]any{"agent": "router"}))

	require.NoError(t, err)

	// Validate return payload
	m, ok := res.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, m["transferred"])
	assert.Equal(t, "router", m["agent"])

	// Validate action set on ToolContext
	actions := tc.EventActions()
	require.NotNil(t, actions)
	assert.Equal(t, "router", actions.TransferToAgent.Or(""))
}

func TestTransferToAgentTool_MissingAgent(t *testing.T) {
	tool, err := NewTransferToAgentTool()
	require.NoError(t, err)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc-1")
	})

	_, err = tool.Call(context.Background(), tc, "")
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
}

func TestTransferToAgentTool_WrongType(t *testing.T) {
	tool, err := NewTransferToAgentTool()
	require.NoError(t, err)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc-1")
	})

	_, err = tool.Call(context.Background(), tc, testutil.MustJSON(t, map[string]any{"agent": 42}))
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
}

func TestTransferToAgentTool_EmptyString(t *testing.T) {
	tool, err := NewTransferToAgentTool()
	require.NoError(t, err)

	tc := core.NewToolContext(dummyRequestContext(), func(o *core.ToolContextOptions) {
		o.FunctionCallID = core.String("fc-1")
	})

	_, err = tool.Call(context.Background(), tc, testutil.MustJSON(t, map[string]any{"agent": ""}))
	require.Error(t, err)
	terr, ok := err.(*Error)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", terr.Code)
}

func TestTransferToAgentTool_ParametersShape(t *testing.T) {
	tool, err := NewTransferToAgentTool()
	require.NoError(t, err)

	params := tool.Parameters()

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	agentSchema, ok := props["agent"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", agentSchema["type"])
}
