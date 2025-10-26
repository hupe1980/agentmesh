package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExampleProvider struct {
	examples []core.Example
	err      error
}

func (m mockExampleProvider) Examples(_ context.Context, _ core.ReadonlyContext) ([]core.Example, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.examples, nil
}

func TestExamples_Static(t *testing.T) {
	example := core.Example{
		Input: []core.Part{
			core.NewPartFromText("static input"),
		},
		Output: []core.Part{
			core.NewPartFromText("static output"),
		},
	}

	examples := core.NewExamples(example)

	assert.True(t, examples.IsStatic(), "expected static examples")

	roCtx := testutil.NewTestRequestContext()

	resolved1, err := examples.Resolve(context.Background(), roCtx)
	require.NoError(t, err)
	require.Len(t, resolved1, 1)
	assertExampleTexts(t, resolved1[0], []string{"static input"}, []string{"static output"})

	// Mutate the resolved copy and ensure a subsequent resolution is unaffected.
	resolved1[0].Input = nil
	resolved1[0].Output = nil

	resolved2, err := examples.Resolve(context.Background(), roCtx)
	require.NoError(t, err)
	require.Len(t, resolved2, 1)
	assertExampleTexts(t, resolved2[0], []string{"static input"}, []string{"static output"})
}

func TestExamples_DynamicFunc(t *testing.T) {
	callCount := 0

	examples := core.NewExamplesFromFunc(func(_ context.Context, _ core.ReadonlyContext) ([]core.Example, error) {
		callCount++
		return []core.Example{{
			Input:  []core.Part{core.NewPartFromText("dynamic input")},
			Output: []core.Part{core.NewPartFromText("dynamic output")},
		}}, nil
	})

	assert.False(t, examples.IsStatic(), "expected dynamic examples")

	roCtx := testutil.NewTestRequestContext()

	resolved1, err := examples.Resolve(context.Background(), roCtx)
	require.NoError(t, err)
	require.Len(t, resolved1, 1)
	assert.Equal(t, 1, callCount)
	assertExampleTexts(t, resolved1[0], []string{"dynamic input"}, []string{"dynamic output"})

	_, err = examples.Resolve(context.Background(), roCtx)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "expected provider to be invoked per Resolve call")
}

func TestExamples_DynamicProvider(t *testing.T) {
	provider := mockExampleProvider{
		examples: []core.Example{{
			Input:  []core.Part{core.NewPartFromText("provider input")},
			Output: []core.Part{core.NewPartFromText("provider output")},
		}},
	}

	examples := core.NewExamplesFromProvider(provider)
	assert.False(t, examples.IsStatic(), "expected dynamic examples")

	resolved, err := examples.Resolve(context.Background(), testutil.NewTestRequestContext())
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	assertExampleTexts(t, resolved[0], []string{"provider input"}, []string{"provider output"})
}

func TestExamples_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("dynamic failure")

	examples := core.NewExamplesFromProvider(mockExampleProvider{err: expectedErr})

	_, err := examples.Resolve(context.Background(), testutil.NewTestRequestContext())
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func assertExampleTexts(t *testing.T, example core.Example, expectedInput, expectedOutput []string) {
	t.Helper()

	require.Equal(t, len(expectedInput), len(example.Input), "unexpected input length")
	require.Equal(t, len(expectedOutput), len(example.Output), "unexpected output length")

	assertTextParts(t, example.Input, expectedInput)
	assertTextParts(t, example.Output, expectedOutput)
}

func assertTextParts(t *testing.T, parts []core.Part, expected []string) {
	t.Helper()

	got := make([]string, len(parts))
	for i, part := range parts {
		textPart, ok := part.(*core.TextPart)
		require.Truef(t, ok, "part %d is %T, expected *core.TextPart", i, part)
		got[i] = textPart.Text
	}

	assert.Equal(t, expected, got)
}
