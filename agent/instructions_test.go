package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	text string
	err  error
}

func (m mockProvider) Instructions(_ context.Context, _ core.ReadonlyContext) (string, error) {
	return m.text, m.err
}

func TestInstruction_Static(t *testing.T) {
	inst := NewInstructionsFromText("static instruction")
	assert.True(t, inst.IsStatic(), "expected static instruction")
	got, err := inst.Resolve(context.Background(), testutil.NewTestRequestContext())
	require.NoError(t, err)
	assert.Equal(t, "static instruction", got)
}

func TestInstruction_NewInstructionFromFunc(t *testing.T) {
	inst := NewInstructionsFromFunc(
		func(_ context.Context, _ core.ReadonlyContext) (string, error) {
			return "dynamic via func", nil
		},
	)

	assert.False(t, inst.IsStatic(), "expected dynamic instruction")

	got, err := inst.Resolve(context.Background(), testutil.NewTestRequestContext())
	require.NoError(t, err)
	assert.Equal(t, "dynamic via func", got)
}

func TestInstruction_NewInstructionFromProvider(t *testing.T) {
	inst := NewInstructionsFromProvider(mockProvider{text: "provider text"})
	assert.False(t, inst.IsStatic(), "expected dynamic instruction")

	got, err := inst.Resolve(context.Background(), testutil.NewTestRequestContext())
	require.NoError(t, err)
	assert.Equal(t, "provider text", got)
}

func TestInstruction_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("boom")
	inst := NewInstructionsFromProvider(mockProvider{err: expectedErr})
	_, err := inst.Resolve(context.Background(), testutil.NewTestRequestContext())
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}
