package flow

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBelongsToBranch(t *testing.T) {
	tests := []struct {
		name         string
		reqCtxBranch string
		evBranch     core.Opt[string]
		want         bool
	}{
		{name: "both empty", reqCtxBranch: "", evBranch: core.String(""), want: true},
		{name: "inv empty, event non-empty", reqCtxBranch: "", evBranch: core.String("parent"), want: true},
		{name: "event nil", reqCtxBranch: "parent.child", evBranch: core.None[string](), want: true},
		{name: "event empty", reqCtxBranch: "parent.child", evBranch: core.String(""), want: true},
		{name: "prefix match", reqCtxBranch: "parent.child", evBranch: core.String("parent"), want: true},
		{name: "exact match", reqCtxBranch: "parent.child", evBranch: core.String("parent.child"), want: true},
		{name: "non-prefix longer event", reqCtxBranch: "parent", evBranch: core.String("parent.child"), want: false},
		{name: "different root", reqCtxBranch: "abc", evBranch: core.String("abd"), want: false},
		{name: "similar root simple-prefix true", reqCtxBranch: "parentX.child", evBranch: core.String("parent"), want: true},
		{
			name:         "deeper invocation under event branch",
			reqCtxBranch: "parent.child.grand",
			evBranch:     core.String("parent.child"),
			want:         true,
		},
		{name: "case sensitive mismatch", reqCtxBranch: "Parent.child", evBranch: core.String("parent"), want: false},
		{name: "exact match root only", reqCtxBranch: "parent", evBranch: core.String("parent"), want: true},
		{
			name:         "event with trailing dot still prefix",
			reqCtxBranch: "parent.child",
			evBranch:     core.String("parent."),
			want:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, belongsToBranch(tt.reqCtxBranch, tt.evBranch)) })
	}
}

func TestMessagesProcessor_MaxAfterFilter(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	mk := func(text string, branch *string) *core.Event {
		ev := core.NewFullAssistantEvent("run1", "agent", core.NewPartFromText(text))
		if branch != nil {
			ev.Branch = core.String(*branch)
		}
		return ev
	}
	bRoot := func(s string) *string { return &s }
	sess.AddEvents(
		mk("u1 root", bRoot("root")),
		mk("a1 root", bRoot("root")),
		mk("u2 other", bRoot("other")),
		mk("a2 root.child", bRoot("root.child")),
		mk("a3 root.child", bRoot("root.child")),
		mk("a4 other", bRoot("other")),
		mk("a5 root.child", bRoot("root.child")),
	)

	agent := testutil.NewMockAgent("m")
	agent.MaxHistoryVal = 2
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "You are a test assistant.", nil
	}

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.UserParts = []core.Part{core.NewPartFromText("hello")}
		p.MaxModelCalls = 10
		p.Session = sess
	}).NewBranchContextForSubAgent("root.child")

	p := NewMessagesProcessor()
	req := &core.ModelRequest{Instructions: "sys"}

	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	assert.Equal(t, 2, len(req.Messages))
	gotTexts := []string{}
	for _, c := range req.Messages {
		for _, part := range c.Parts {
			if tp, ok := part.(*core.TextPart); ok {
				gotTexts = append(gotTexts, tp.Text)
			}
		}
	}
	// Since the author of the events is "agent" and current agent is "m",
	// these are treated as other-agent messages and transformed for context.
	assert.Equal(t, []string{
		"For context:", "[agent] said: a3 root.child",
		"For context:", "[agent] said: a5 root.child",
	}, gotTexts)
}

func TestMessagesProcessor_OtherAgentTextTransformed(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Other agent sends a text
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("hello from other")))
	// Current agent also has a reply; only to ensure mix
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("own message")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 10

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	// Expect two messages in chronological order; first transformed "other" context, then current
	require.Len(t, req.Messages, 2)
	// Message 1 is user role with contextual text
	assert.Equal(t, core.RoleUser, req.Messages[0].Role)
	parts0 := partsToStrings(req.Messages[0].Parts)
	assert.Equal(t, []string{"For context:", "[other] said: hello from other"}, parts0)
	// Message 2 preserves current agent's own content/role
	assert.Equal(t, core.RoleAssistant, req.Messages[1].Role)
	parts1 := partsToStrings(req.Messages[1].Parts)
	assert.Equal(t, []string{"own message"}, parts1)
}

func TestMessagesProcessor_OtherAgentFunctionCallTransformed(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Other agent made a tool call
	ev := core.NewFullAssistantEvent("run1", "other",
		core.NewPartFromFunctionCall("c1", "echo", "{\"x\":1}"))
	sess.AddEvents(ev)

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 5

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	assert.Equal(t, core.RoleUser, req.Messages[0].Role)
	parts := partsToStrings(req.Messages[0].Parts)
	// Parameters are stringified via fmt.Sprint; exact formatting should include the JSON string
	assert.Contains(t, parts, "For context:")
	assert.Contains(t, parts, "[other] called tool `echo` with parameters: {\"x\":1}")
}

func TestMessagesProcessor_OtherAgentFunctionResponseTransformed(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Other agent produced a tool response
	ev := core.NewFullAssistantEvent("run1", "other",
		core.NewPartFromFunctionResponse("c1", "echo", map[string]any{"ok": true}))
	sess.AddEvents(ev)

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 5

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	assert.Equal(t, core.RoleUser, req.Messages[0].Role)
	parts := partsToStrings(req.Messages[0].Parts)
	assert.Contains(t, parts, "For context:")
	// Response is stringified; assert substring
	assert.Contains(t, parts[1], "[other] `echo` tool returned result:")
	assert.Contains(t, parts[1], "map[ok:true]")
}

func TestMessagesProcessor_SkipsEmptyOtherAgentMessage(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Event with no parts is ignored
	empty := core.NewFullAssistantEvent("run1", "other")
	empty.Parts = nil
	sess.AddEvents(empty)

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 5

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)
	// No messages selected
	assert.Len(t, req.Messages, 0)
}

func TestMessagesProcessor_RespectsChronologicalOrder(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("first")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("second")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 10

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	require.Len(t, req.Messages, 2)
	first := partsToStrings(req.Messages[0].Parts)
	second := partsToStrings(req.Messages[1].Parts)
	assert.Equal(t, []string{"For context:", "[other] said: first"}, first)
	assert.Equal(t, []string{"For context:", "[other] said: second"}, second)
}

func TestMessagesProcessor_MixedWithLimit(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Oldest to newest
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("b1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a2")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("b2")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 3

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	// We should get the last 3 after filtering/transforming, in order.
	// 1) [other] b1 (transformed) is older and should be dropped without limit,
	//    but with limit we reverse the last 3 to chronological.
	// From newest backwards: other b2, current a2, other b1, current a1.
	// We collect 3: other b2 (transformed), current a2, other b1 (transformed).
	// After reverse to chronological: other b1, current a2, other b2.
	// Since limit=3, we expect exactly these three.
	require.Len(t, req.Messages, 3)
	got := [][]string{
		partsToStrings(req.Messages[0].Parts),
		partsToStrings(req.Messages[1].Parts),
		partsToStrings(req.Messages[2].Parts),
	}
	assert.Equal(t, []string{"For context:", "[other] said: b1"}, got[0])
	assert.Equal(t, []string{"a2"}, got[1])
	assert.Equal(t, []string{"For context:", "[other] said: b2"}, got[2])
}

func TestMessagesProcessor_HistoryNone_NoSelectionIfNewestIsSelf(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Oldest to newest
	sess.AddEvents(core.NewUserContentEvent("run1", core.NewPartFromText("u1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a1")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 10
	agent.HistoryModeVal = core.HistoryNone

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	// Since the newest event is authored by the current agent, HistoryNone stops immediately
	// without selecting any message.
	assert.Len(t, req.Messages, 0)
}

func TestMessagesProcessor_HistoryNone_SelectsLatestUser(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Oldest to newest
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a1")))
	sess.AddEvents(core.NewUserContentEvent("run1", core.NewPartFromText("u2")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 10
	agent.HistoryModeVal = core.HistoryNone

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	assert.Equal(t, core.RoleUser, req.Messages[0].Role)
	assert.Equal(t, []string{"u2"}, partsToStrings(req.Messages[0].Parts))
}

func TestMessagesProcessor_HistoryNone_SelectsLatestOtherAsContext(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Oldest to newest
	sess.AddEvents(core.NewUserContentEvent("run1", core.NewPartFromText("u1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("b1")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 10
	agent.HistoryModeVal = core.HistoryNone

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	assert.Equal(t, core.RoleUser, req.Messages[0].Role)
	assert.Equal(t, []string{"For context:", "[other] said: b1"}, partsToStrings(req.Messages[0].Parts))
}

func TestMessagesProcessor_HistoryOwn_OnlyUserAndSelfWithLimit(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Oldest to newest
	sess.AddEvents(core.NewUserContentEvent("run1", core.NewPartFromText("u1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("b1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a1")))
	sess.AddEvents(core.NewUserContentEvent("run1", core.NewPartFromText("u2")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("b2")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a2")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 3
	agent.HistoryModeVal = core.HistoryOwn

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	// Expect the last 3 among only user/self: from newest backwards => a2, u2, a1
	// After reverse (chronological): a1, u2, a2
	require.Len(t, req.Messages, 3)
	assert.Equal(t, []string{"a1"}, partsToStrings(req.Messages[0].Parts))
	assert.Equal(t, []string{"u2"}, partsToStrings(req.Messages[1].Parts))
	assert.Equal(t, []string{"a2"}, partsToStrings(req.Messages[2].Parts))

	// Ensure roles are preserved (no transformed context messages)
	assert.Equal(t, core.RoleAssistant, req.Messages[0].Role)
	assert.Equal(t, core.RoleUser, req.Messages[1].Role)
	assert.Equal(t, core.RoleAssistant, req.Messages[2].Role)
}

func TestMessagesProcessor_HistoryAll_IncludesUserAndTransformsOthers(t *testing.T) {
	sess := core.NewSession("app", "user1", "sess1")
	// Oldest to newest
	sess.AddEvents(core.NewUserContentEvent("run1", core.NewPartFromText("u1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "other", core.NewPartFromText("b1")))
	sess.AddEvents(core.NewFullAssistantEvent("run1", "current", core.NewPartFromText("a1")))

	agent := testutil.NewMockAgent("current")
	agent.MaxHistoryVal = 10
	agent.HistoryModeVal = core.HistoryAll

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = agent
		p.RunID = "r1"
		p.Session = sess
	})

	p := NewMessagesProcessor()
	req := &core.ModelRequest{}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	require.Len(t, req.Messages, 3)
	// Chronological order
	assert.Equal(t, []string{"u1"}, partsToStrings(req.Messages[0].Parts))
	assert.Equal(t, []string{"For context:", "[other] said: b1"}, partsToStrings(req.Messages[1].Parts))
	assert.Equal(t, []string{"a1"}, partsToStrings(req.Messages[2].Parts))

	// Role check: user, user (context), assistant
	assert.Equal(t, core.RoleUser, req.Messages[0].Role)
	assert.Equal(t, core.RoleUser, req.Messages[1].Role)
	assert.Equal(t, core.RoleAssistant, req.Messages[2].Role)
}

// Helper to extract text parts for assertions
func partsToStrings(parts []core.Part) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if tp, ok := p.(*core.TextPart); ok {
			out = append(out, tp.Text)
		}
	}

	return out
}
