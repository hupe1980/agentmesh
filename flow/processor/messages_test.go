package processor

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
		{name: "deeper invocation under event branch",
			reqCtxBranch: "parent.child.grand",
			evBranch:     core.String("parent.child"),
			want:         true,
		},
		{name: "case sensitive mismatch", reqCtxBranch: "Parent.child", evBranch: core.String("parent"), want: false},
		{name: "exact match root only", reqCtxBranch: "parent", evBranch: core.String("parent"), want: true},
		{name: "event with trailing dot still prefix",
			reqCtxBranch: "parent.child",
			evBranch:     core.String("parent."),
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := belongsToBranch(tt.reqCtxBranch, tt.evBranch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMessagesProcessor_MaxAfterFilter(t *testing.T) {
	// Build a session history with mixed branches; only branch "root.child" should match invocation
	sess := core.NewSession("app", "user1", "sess1")
	// Helper to build content
	mk := func(text string, branch *string) *core.Event {
		ev := core.NewFullAssistantEvent("run1", "agent", core.NewPartFromText(text))
		if branch != nil {
			ev.Branch = core.String(*branch)
		}
		return ev
	}
	bRoot := func(s string) *string { return &s }
	// Events in chronological order
	sess.AddEvents(
		mk("u1 root", bRoot("root")),
		mk("a1 root", bRoot("root")),
		mk("u2 other", bRoot("other")),
		mk("a2 root.child", bRoot("root.child")),
		mk("a3 root.child", bRoot("root.child")),
		mk("a4 other", bRoot("other")),
		mk("a5 root.child", bRoot("root.child")),
	)

	// Request context on branch root.child with max 2 messages
	ag := testutil.NewMockAgent("agent")
	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = ag
		p.RunID = "r1"
		p.UserParts = []core.Part{core.NewPartFromText("hello")}
		p.MaxModelCalls = 10
		p.Session = sess
	})

	// Manually set branch on request context via sub-agent branch helper
	reqCtx = reqCtx.NewBranchContextForSubAgent("root.child")

	agent := testutil.NewMockAgent("m")
	agent.MaxHistoryVal = 2
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "You are a test assistant.", nil
	}

	p := NewMessagesProcessor()
	req := &core.ModelRequest{Instructions: "sys"}

	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)

	// Expect: last 2 matching assistant/user events from branch root.child => a3, a5
	// (chronological restored: a3, a5)
	assert.Equal(t, 2, len(req.Messages))
	gotTexts := []string{}
	for _, c := range req.Messages { // skip system
		for _, part := range c.Parts {
			if tp, ok := part.(*core.TextPart); ok {
				gotTexts = append(gotTexts, tp.Text)
				continue
			}
		}
	}

	assert.Equal(t, []string{"a3 root.child", "a5 root.child"}, gotTexts)
}
