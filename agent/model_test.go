package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LLM Agent Test Cases
func TestModelAgent_NewAgent(t *testing.T) {
	mockLLM := &testutil.MockModel{}
	agent, err := NewModelAgent("Test Agent", mockLLM, testutil.NewMockFlowSelector())
	require.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, mockLLM, agent.model)
	assert.NotNil(t, agent.tools)
	assert.Equal(t, 0, len(agent.Tools()), "no tools registered initially")
	assert.True(t, agent.enableStreaming)
}

func TestModelAgent_ResolveInstructions(t *testing.T) {
	inst := core.NewInstructionsFromText("custom inst")
	a, err := NewModelAgent(
		"AgentX",
		nil,
		testutil.NewMockFlowSelector(),
		func(o *ModelAgentOptions) { o.Instructions = inst },
	)
	require.NoError(t, err)

	ctx := context.Background()
	ro := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	got, err := a.ResolveInstructions(ctx, ro)
	assert.NoError(t, err)
	assert.Equal(t, "custom inst", got)
}

func TestAttachOutputToEvent_AuthorMismatch(t *testing.T) {
	a, err := NewModelAgent(
		"AgentA",
		nil,
		testutil.NewMockFlowSelector(),
		func(o *ModelAgentOptions) { o.OutputKey = "out" },
	)
	require.NoError(t, err)

	req := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	ev := core.NewFullAssistantEvent(req.RunID(), "OtherAgent", core.NewPartFromText("hello"))

	a.attachOutputToEvent(ev)

	_, set := ev.Actions.StateDelta.Get()
	assert.False(t, set, "state delta should not be set when author mismatches")
}

func TestAttachOutputToEvent_NoOutputKey(t *testing.T) {
	a, err := NewModelAgent("AgentA", nil, testutil.NewMockFlowSelector()) // no OutputKey
	require.NoError(t, err)

	req := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	ev := core.NewFullAssistantEvent(req.RunID(), a.Name(), core.NewPartFromText("hello"))

	a.attachOutputToEvent(ev)

	_, set := ev.Actions.StateDelta.Get()
	assert.False(t, set, "state delta should not be set without output key")
}

func TestAttachOutputToEvent_NotFinal(t *testing.T) {
	a, err := NewModelAgent(
		"AgentA",
		nil,
		testutil.NewMockFlowSelector(),
		func(o *ModelAgentOptions) { o.OutputKey = "out" },
	)
	require.NoError(t, err)

	req := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	ev := core.NewFullAssistantEvent(req.RunID(), req.AgentName(), core.NewPartFromText("partial"))
	ev.Partial = core.Bool(true) // mark non-final

	a.attachOutputToEvent(ev)

	_, set := ev.Actions.StateDelta.Get()
	assert.False(t, set, "state delta should not be set for non-final events")
}

func TestAttachOutputToEvent_NoParts(t *testing.T) {
	a, err := NewModelAgent(
		"AgentA",
		nil,
		testutil.NewMockFlowSelector(),
		func(o *ModelAgentOptions) { o.OutputKey = "out" },
	)
	require.NoError(t, err)

	req := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	// No parts
	ev := core.NewFullAssistantEvent(req.RunID(), a.Name())
	ev.Parts = nil

	a.attachOutputToEvent(ev)

	_, set := ev.Actions.StateDelta.Get()
	assert.False(t, set, "state delta should not be set when there are no parts")
}

func TestAttachOutputToEvent_AggregatesText(t *testing.T) {
	a, err := NewModelAgent(
		"AgentA",
		nil,
		testutil.NewMockFlowSelector(),
		func(o *ModelAgentOptions) { o.OutputKey = "out" },
	)
	require.NoError(t, err)

	req := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	ev := core.NewFullAssistantEvent(req.RunID(), a.Name(),
		core.NewPartFromText("Hello, "),
		core.NewPartFromText("World!"),
	)

	a.attachOutputToEvent(ev)

	sd := ev.Actions.StateDelta.Or(nil)
	if assert.NotNil(t, sd, "state delta should be present") {
		assert.Equal(t, "Hello, World!", sd["out"])
	}
}

func TestAttachOutputToEvent_PreservesExistingState(t *testing.T) {
	a, err := NewModelAgent(
		"AgentA",
		nil,
		testutil.NewMockFlowSelector(),
		func(o *ModelAgentOptions) { o.OutputKey = "out" },
	)
	require.NoError(t, err)

	req := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = a
	})

	ev := core.NewFullAssistantEvent(req.RunID(), a.Name(), core.NewPartFromText("val"))
	ev.Actions.StateDelta = core.Map(map[string]any{"prev": 123})

	a.attachOutputToEvent(ev)

	sd := ev.Actions.StateDelta.Or(nil)
	if assert.NotNil(t, sd) {
		assert.Equal(t, 123, sd["prev"])
		assert.Equal(t, "val", sd["out"])
	}
}
