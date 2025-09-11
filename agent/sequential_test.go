package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
)

// SequentialAgent Test Cases
func TestNewSequentialAgent(t *testing.T) {
	child1 := newMockAgent("Child 1", nil)
	child2 := newMockAgent("Child 2", nil)

	agent := NewSequentialAgent("Sequential Agent", []core.Agent{child1, child2})

	assert.NotNil(t, agent)
	assert.Equal(t, "Sequential Agent", agent.Name())
	subs := agent.SubAgents()
	assert.Len(t, subs, 2)
	assert.Equal(t, child1, subs[0])
	assert.Equal(t, child2, subs[1])
}

func TestSequentialAgent_Run_Success(t *testing.T) {
	child1 := newMockAgent(
		"Child 1",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)
	child2 := newMockAgent(
		"Child 2",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)
	child3 := newMockAgent(
		"Child 3",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)

	agent := NewSequentialAgent("Sequential Agent", []core.Agent{child1, child2, child3})

	ctx := context.Background()

	sess := core.NewSession("app", "user1", "sess1")
	agentInfo := core.AgentInfo{
		Name: "Sequential Agent",
		Type: "sequential",
	}

	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:         "run_id",
		Agent:         agentInfo,
		UserParts:     []core.Part{core.TextPart{Text: "test input"}},
		MaxModelCalls: 100,
		Session:       sess,
		SessionStore:  nil,
		ArtifactStore: nil,
		MemoryStore:   nil,
	})

	err := agent.Run(ctx, reqCtx, testutil.DiscardingWriter{})

	assert.NoError(t, err)
}

func TestSequentialAgent_Run_FirstChildError(t *testing.T) {
	sentinel := errors.New("boom")
	child1 := newMockAgent(
		"Child 1",
		func(context.Context, core.RequestContext, core.EventWriter) error {
			return sentinel
		},
	)
	child2 := newMockAgent(
		"Child 2",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)

	agent := NewSequentialAgent("Sequential Agent", []core.Agent{child1, child2})

	ctx := context.Background()
	sess := core.NewSession("app", "user1", "sess1")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:         "run_id",
		Agent:         core.AgentInfo{Name: "Sequential Agent", Type: "sequential"},
		UserParts:     []core.Part{core.TextPart{Text: "test input"}},
		MaxModelCalls: 100,
		Session:       sess,
	})

	err := agent.Run(ctx, reqCtx, testutil.DiscardingWriter{})

	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel) // Check that the original error is wrapped
}

func TestSequentialAgent_Run_NoChildren(t *testing.T) {
	agent := NewSequentialAgent("Sequential Agent", []core.Agent{})

	ctx := context.Background()
	sess := core.NewSession("app", "user1", "sess1")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:         "run_id",
		Agent:         core.AgentInfo{Name: "Sequential Agent", Type: "sequential"},
		UserParts:     []core.Part{core.TextPart{Text: "test input"}},
		MaxModelCalls: 100,
		Session:       sess,
	})

	err := agent.Run(ctx, reqCtx, testutil.DiscardingWriter{})
	assert.NoError(t, err)
}

func TestSequentialAgent_ContextPropagation(t *testing.T) {
	child1 := newMockAgent(
		"Child 1",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)
	child2 := newMockAgent(
		"Child 2",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)

	agent := NewSequentialAgent("Sequential Agent", []core.Agent{child1, child2})

	ctx := context.Background()
	sess := core.NewSession("app", "user1", "sess1")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:         "run_id",
		Agent:         core.AgentInfo{Name: "Sequential Agent", Type: "sequential"},
		UserParts:     []core.Part{core.TextPart{Text: "test input"}},
		MaxModelCalls: 100,
		Session:       sess,
	})

	err := agent.Run(ctx, reqCtx, testutil.DiscardingWriter{})

	assert.NoError(t, err)
	assert.Equal(t, reqCtx.RunID(), child1.ReceivedCtx().RunID())
	assert.Equal(t, reqCtx.SessionID(), child1.ReceivedCtx().SessionID())
	assert.Equal(t, reqCtx.RunID(), child2.ReceivedCtx().RunID())
	assert.Equal(t, reqCtx.SessionID(), child2.ReceivedCtx().SessionID())
}
