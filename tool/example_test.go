package tool

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRenderExamples_DefaultTemplate(t *testing.T) {
	opts := defaultExampleToolOptions()
	examples := []core.Example{
		{
			Input:  []core.Part{core.NewPartFromText("hello")},
			Output: []core.Part{core.NewPartFromText("world")},
		},
	}

	rendered, err := RenderExamples(opts, examples)
	require.NoError(t, err)

	expected := `<examples>
[user]
hello
[assistant]
world
</examples>
`
	require.Equal(t, expected, rendered)
}

func TestRenderExamples_CustomTemplate(t *testing.T) {
	opts := defaultExampleToolOptions()
	opts.Template = "{{ range .Examples }}{{ .Number }}: {{ .Input }} -> {{ .Output }}; {{ end }}"
	opts.ExamplesIntro = ""
	opts.ExamplesEnd = ""
	opts.UserPrefix = ""
	opts.AssistantPrefix = ""

	examples := []core.Example{
		{
			Input:  []core.Part{core.NewPartFromText("ping")},
			Output: []core.Part{core.NewPartFromText("pong")},
		},
		{
			Input:  []core.Part{core.NewPartFromText("left")},
			Output: []core.Part{core.NewPartFromText("right")},
		},
	}

	rendered, err := RenderExamples(opts, examples)
	require.NoError(t, err)

	expected := "1: ping -> pong; 2: left -> right; "
	require.Equal(t, expected, rendered)
}

func TestRenderExamples_DefaultTemplateMultipleExamples(t *testing.T) {
	opts := defaultExampleToolOptions()

	examples := []core.Example{
		{
			Input:  []core.Part{core.NewPartFromText("one")},
			Output: []core.Part{core.NewPartFromText("uno")},
		},
		{
			Input:  []core.Part{core.NewPartFromText("two")},
			Output: []core.Part{core.NewPartFromText("dos")},
		},
	}

	rendered, err := RenderExamples(opts, examples)
	require.NoError(t, err)

	expected := `<examples>
[user]
one
[assistant]
uno

[user]
two
[assistant]
dos
</examples>
`
	require.Equal(t, expected, rendered)
}

func TestRenderExamples_FunctionCallOutput(t *testing.T) {
	opts := defaultExampleToolOptions()

	examples := []core.Example{
		{
			Input: []core.Part{
				core.NewPartFromText("What's the weather in Berlin for the next 3 days?"),
			},
			Output: []core.Part{
				&core.FunctionCallPart{
					FunctionCall: &core.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Berlin","days":3}`,
					},
				},
			},
		},
	}

	rendered, err := RenderExamples(opts, examples)
	require.NoError(t, err)

	expected := "<examples>\n[user]\n" +
		"What's the weather in Berlin for the next 3 days?\n" +
		"[assistant]\n" +
		opts.FunctionCallPrefix +
		"get_weather(city='Berlin', days=3)" +
		opts.FunctionCallSuffix +
		"\n</examples>\n"
	require.Equal(t, expected, rendered)
}

func TestRenderExamples_FunctionCallOutput_CustomWrapper(t *testing.T) {
	opts := defaultExampleToolOptions()
	opts.FunctionCallPrefix = ""
	opts.FunctionCallSuffix = ""

	examples := []core.Example{
		{
			Input: []core.Part{
				core.NewPartFromText("What's the weather in Berlin for the next 3 days?"),
			},
			Output: []core.Part{
				&core.FunctionCallPart{
					FunctionCall: &core.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Berlin","days":3}`,
					},
				},
			},
		},
	}

	rendered, err := RenderExamples(opts, examples)
	require.NoError(t, err)

	expected := `<examples>
[user]
What's the weather in Berlin for the next 3 days?
[assistant]
get_weather(city='Berlin', days=3)
</examples>
`
	require.Equal(t, expected, rendered)
}

func TestRenderExamples_UnsupportedPart(t *testing.T) {
	opts := defaultExampleToolOptions()

	examples := []core.Example{
		{
			Output: []core.Part{
				core.NewPartFromFunctionResponse("call-id", "get_weather", map[string]any{"temp": 21}),
			},
		},
	}

	_, err := RenderExamples(opts, examples)
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported example part type *core.FunctionResponsePart")
}

func TestExampleTool_ProcessModelRequest_UsesRenderer(t *testing.T) {
	provider := core.ExampleProviderFunc(func(_ context.Context, _ core.ReadonlyContext) ([]core.Example, error) {
		return []core.Example{
			{
				Input:  []core.Part{core.NewPartFromText("alpha")},
				Output: []core.Part{core.NewPartFromText("beta")},
			},
		}, nil
	})

	tool := NewExampleTool(provider)
	req := &core.ModelRequest{}
	tc := core.NewToolContext(testutil.NewTestRequestContext())

	err := tool.ProcessModelRequest(context.Background(), tc, req)
	require.NoError(t, err)

	expected, err := RenderExamples(tool.opts, []core.Example{
		{
			Input:  []core.Part{core.NewPartFromText("alpha")},
			Output: []core.Part{core.NewPartFromText("beta")},
		},
	})
	require.NoError(t, err)

	require.Equal(t, expected, req.Instructions)
}

func TestExampleTool_ProcessModelRequest_UnsupportedPart(t *testing.T) {
	provider := core.ExampleProviderFunc(func(_ context.Context, _ core.ReadonlyContext) ([]core.Example, error) {
		return []core.Example{
			{
				Output: []core.Part{
					core.NewPartFromFunctionResponse("call-id", "get_weather", nil),
				},
			},
		}, nil
	})

	tool := NewExampleTool(provider)
	req := &core.ModelRequest{}
	tc := core.NewToolContext(testutil.NewTestRequestContext())

	err := tool.ProcessModelRequest(context.Background(), tc, req)
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported example part type *core.FunctionResponsePart")
}
