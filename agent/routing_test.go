package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingAgent_SelectorNil(t *testing.T) {
	ag := NewRoutingAgent("conditional", nil, nil)

	err := ag.Run(context.Background(), testutil.NewTestRequestContext(), testutil.DiscardingWriter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRoutingSelectorNil)
}

func TestRoutingAgent_NoSubAgents(t *testing.T) {
	selector := func(ctx context.Context, roCtx core.ReadonlyContext, agents []core.Agent) (string, error) {
		return "child", nil
	}

	ag := NewRoutingAgent("conditional", nil, selector)

	err := ag.Run(context.Background(), testutil.NewTestRequestContext(), testutil.DiscardingWriter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRoutingNoAgents)
}

func TestRoutingAgent_InvalidSelection(t *testing.T) {
	selector := func(ctx context.Context, roCtx core.ReadonlyContext, agents []core.Agent) (string, error) {
		return "missing", nil
	}

	child := NewFuncAgent("child", func(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
		return nil
	})
	ag := NewRoutingAgent("conditional", []core.Agent{child}, selector)

	err := ag.Run(context.Background(), testutil.NewTestRequestContext(), testutil.DiscardingWriter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRoutingInvalidChoice)
}

func TestRoutingAgent_SelectorError(t *testing.T) {
	selector := func(ctx context.Context, roCtx core.ReadonlyContext, agents []core.Agent) (string, error) {
		return "", assert.AnError
	}

	child := NewFuncAgent("child", func(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
		return nil
	})
	ag := NewRoutingAgent("conditional", []core.Agent{child}, selector)

	err := ag.Run(context.Background(), testutil.NewTestRequestContext(), testutil.DiscardingWriter{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "routing selection failed")
}

func TestRoutingAgent_ExecutesSelectedChild(t *testing.T) {
	executed := false

	child := NewFuncAgent("child", func(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
		executed = true
		return nil
	})

	selector := func(ctx context.Context, roCtx core.ReadonlyContext, agents []core.Agent) (string, error) {
		return "child", nil
	}

	ag := NewRoutingAgent("conditional", []core.Agent{child}, selector)

	err := ag.Run(context.Background(), testutil.NewTestRequestContext(), testutil.DiscardingWriter{})

	require.NoError(t, err)
	assert.True(t, executed, "selected child should have executed")
}
