package flow

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicProcessor_UsesNativeStructuredOutput(t *testing.T) {
	agent := testutil.NewMockAgent("agent")
	agent.ModelVal = &testutil.MockModel{
		CapabilitiesVal: core.ModelCapabilities{SupportsStructuredOutput: true},
	}

	agent.OutputSchemaVal = core.MustNewOutputSchema("answer", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
		"required": []string{"answer"},
	})

	req := &core.ModelRequest{}

	proc := NewOutputSchemaProcessor()
	err := proc.ProcessRequest(context.Background(), newTestRunContext(), req, agent)
	require.NoError(t, err)

	assert.True(t, req.OutputSchema.IsSet())
	assert.Empty(t, req.Tools)
}

func TestBasicProcessor_AttachesFallbackTool(t *testing.T) {
	agent := testutil.NewMockAgent("agent")
	agent.ModelVal = &testutil.MockModel{}
	agent.OutputSchemaVal = core.MustNewOutputSchema("answer", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
		"required": []string{"answer"},
	})

	req := &core.ModelRequest{}

	proc := NewOutputSchemaProcessor()
	err := proc.ProcessRequest(context.Background(), newTestRunContext(), req, agent)
	require.NoError(t, err)

	assert.False(t, req.OutputSchema.IsSet())
	if assert.NotNil(t, req.ToolRegistry) {
		_, exists := req.ToolRegistry["set_model_response"]
		assert.True(t, exists)
	}
	require.Len(t, req.Tools, 1)
	assert.Equal(t, "set_model_response", req.Tools[0].Function.Name)
}
