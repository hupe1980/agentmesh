package flow

import (
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeterministicMerge_ByOriginalCallOrder(t *testing.T) {
	// Original call order: c1, c2, c3
	fnCalls := []*core.FunctionCall{
		{ID: "c1", Name: "t1"},
		{ID: "c2", Name: "t2"},
		{ID: "c3", Name: "t3"},
	}

	// Collected tool events arrive out-of-order: c2 then c1
	ev2 := core.NewFunctionResponseEvent("run_id", "agent", "c2", "t2", map[string]any{"ok": true})
	ev2.Actions.StateDelta = core.Map(map[string]any{"a": 2, "b": 1})
	ev2.Actions.ArtifactDelta = core.Map(map[string]int{"x": 2})
	ev2.Actions.Escalate = core.Some(true)
	ev2.Actions.SkipSummarization = core.Some(true)

	ev1 := core.NewFunctionResponseEvent("run_id", "agent", "c1", "t1", "res1")
	ev1.Actions.StateDelta = core.Map(map[string]any{"a": 1})
	ev1.Actions.ArtifactDelta = core.Map(map[string]int{"x": 1})
	ev1.Actions.TransferToAgent = core.Some("agentA")

	collected := []*core.Event{ev2, ev1}

	// Index and choose template by original order
	respByID, actionsByID := indexByCallID(collected)
	tmpl := chooseTemplateResponse(fnCalls, respByID)
	require.NotNil(t, tmpl)
	assert.Equal(t, "c1", tmpl.ID, "template should be the first call with a response (c1)")

	// Build parts in order (c1 then c2)
	parts := buildPartsInOrder(fnCalls, respByID)
	require.Len(t, parts, 2)
	p1 := parts[0].(*core.FunctionResponsePart)
	p2 := parts[1].(*core.FunctionResponsePart)
	assert.Equal(t, "c1", p1.FunctionResponse.ID)
	assert.Equal(t, "c2", p2.FunctionResponse.ID)

	// Merge actions deterministically by call order
	sd, ad, transfer, escalate, skip := mergeActionsInOrder(fnCalls, actionsByID)

	// StateDelta: last write wins (c2 overwrites key 'a')
	assert.Equal(t, any(2), sd["a"]) // from c2
	assert.Equal(t, any(1), sd["b"]) // from c2
	// ArtifactDelta: last write wins
	assert.Equal(t, 2, ad["x"]) // from c2
	// First-set-wins for Transfer/Escalate based on original order
	assert.Equal(t, "agentA", transfer.Or(""))
	assert.True(t, escalate.Or(false))
	// Skip summarization OR-reduced
	assert.True(t, skip.Or(false))
}
