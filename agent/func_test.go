package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuncAgent_Run_CallsFunc(t *testing.T) {
	called := false
	a := NewFuncAgent("fn", func(ctx context.Context, _ core.RequestContext, _ core.EventWriter) error {
		called = true
		return nil
	})

	err := a.Run(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.True(t, called, "run function should be called")
}

func TestFuncAgent_Run_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	a := NewFuncAgent(
		"fn",
		func(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
			return want
		},
	)

	got := a.Run(context.Background(), nil, nil)
	require.Error(t, got)
	assert.Equal(t, want, got)
}

func TestFuncAgent_BaseAgentFields(t *testing.T) {
	a := NewFuncAgent(
		"worker",
		func(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
			return nil
		},
		func(o *FuncAgentOptions) {
			o.Description = "Agent worker"
		},
	)
	assert.Equal(t, "worker", a.Name())
	assert.Equal(t, "Agent worker", a.Description())
}

func TestFuncAgent_Run_NilFunc_Error(t *testing.T) {
	// Constructor allows nil, run should error lazily
	a := NewFuncAgent("nil", nil)
	err := a.Run(context.Background(), nil, nil)
	assert.ErrorIs(t, err, ErrRunFuncNil)
}
