package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/require"
)

type toolAwareAgent struct {
	*testutil.MockAgent
}

func (a *toolAwareAgent) Tools() []core.Tool {
	return a.ToolsList
}

type stubTool struct {
	name string
	desc string
}

func (t stubTool) Name() string               { return t.name }
func (t stubTool) Description() string        { return t.desc }
func (t stubTool) Parameters() map[string]any { return map[string]any{} }
func (t stubTool) ProcessModelRequest(ctx context.Context, toolCtx core.ToolContext, req *core.ModelRequest) error {
	return nil
}
func (t stubTool) Call(ctx context.Context, toolCtx core.ToolContext, args string) (any, error) {
	return nil, nil
}

func TestFlowchart(t *testing.T) {
	root := &toolAwareAgent{MockAgent: testutil.NewMockAgent("Root Agent")}
	child := &toolAwareAgent{MockAgent: testutil.NewMockAgent("Child Agent")}
	leaf := testutil.NewMockAgent("Leaf")

	require.NoError(t, root.AddSubAgents(child))
	require.NoError(t, child.AddSubAgents(leaf))

	root.DescriptionVal = "Orchestrates the workflow"
	root.ToolsList = []core.Tool{
		stubTool{name: "web_search", desc: "Search the web"},
	}

	chart, err := Flowchart(root, WithDirection("LR"))
	require.NoError(t, err)

	require.Contains(t, chart, "flowchart LR")
	require.Contains(t, chart, "Root_Agent[\"Root Agent")
	require.Contains(t, chart, "Child_Agent[\"Child Agent")
	require.Contains(t, chart, "Leaf[\"Leaf")
	require.Contains(t, chart, "Root_Agent --> Child_Agent")
	require.Contains(t, chart, "Child_Agent --> Leaf")
	require.Contains(t, chart, "Root_Agent -.->|uses| Root_Agent_tool_0")
	require.Contains(t, chart, "Root_Agent_tool_0[[\"Tool: web_search")
}

func TestFlowchartNilRoot(t *testing.T) {
	chart, err := Flowchart(nil)
	require.Error(t, err)
	require.Empty(t, chart)
}
