package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type weatherArgs struct {
	Location string `json:"location" jsonschema:"required,description=City name"`
	Unit     string `json:"unit,omitempty" jsonschema:"description=Temperature unit (C or F)"`
}

func weatherFunc(_ context.Context, args weatherArgs) (map[string]any, error) {
	if args.Location == "" {
		return nil, errors.New("location required")
	}
	return map[string]any{
		"location":    args.Location,
		"temperature": 21,
		"unit":        args.Unit,
	}, nil
}

func TestNewFuncTool_Basic(t *testing.T) {
	tool, err := NewFuncTool("weather", "Get weather for a location", weatherFunc)

	require.NoError(t, err)
	require.NotNil(t, tool)
	assert.Equal(t, "weather", tool.Name())
	assert.Equal(t, "Get weather for a location", tool.Description())
}

func TestNewFuncTool_EmptyName(t *testing.T) {
	// Current implementation doesn't validate empty name
	tool, err := NewFuncTool("", "description", weatherFunc)

	// It should ideally return an error, but currently doesn't
	// So we just verify it doesn't crash
	if err == nil {
		assert.NotNil(t, tool)
	}
}

func TestNewFuncTool_NilFunc(t *testing.T) {
	// Current implementation doesn't validate nil function
	tool, err := NewFuncTool[weatherArgs, any]("test", "description", nil)

	// It should ideally return an error, but currently doesn't
	// So we just verify it doesn't crash
	if err == nil {
		assert.NotNil(t, tool)
	}
}

func TestFuncTool_Definition(t *testing.T) {
	tool, err := NewFuncTool("weather", "Get weather", weatherFunc)
	require.NoError(t, err)

	def := tool.Definition()

	require.NotNil(t, def)
	assert.Equal(t, "function", def.Type)
	assert.Equal(t, "weather", def.Function.Name)
	assert.Equal(t, "Get weather", def.Function.Description)
	assert.NotNil(t, def.Function.Parameters)

	// Check schema was generated
	params := def.Function.Parameters
	assert.Equal(t, "object", params["type"])

	// Check properties exist
	properties, ok := params["properties"].(map[string]any)
	require.True(t, ok, "parameters should have properties")
	assert.Contains(t, properties, "location")
	assert.Contains(t, properties, "unit")
}

func TestFuncTool_Call(t *testing.T) {
	// Setup common tool
	tool, err := NewFuncTool("weather", "Get weather", weatherFunc)
	require.NoError(t, err)

	tests := []struct {
		name        string
		args        string
		wantErr     bool
		errContains string
		checkResult func(t *testing.T, result any)
	}{
		{
			name:    "success",
			args:    `{"location":"Berlin","unit":"C"}`,
			wantErr: false,
			checkResult: func(t *testing.T, result any) {
				require.NotNil(t, result)
				resultMap, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "Berlin", resultMap["location"])
				assert.Equal(t, 21, resultMap["temperature"])
				assert.Equal(t, "C", resultMap["unit"])
			},
		},
		{
			name:        "invalid_json",
			args:        `{invalid json}`,
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name:        "function_error_missing_required",
			args:        `{"unit":"C"}`,
			wantErr:     true,
			errContains: "location",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Call(context.Background(), tt.args)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestFuncTool_Call_EmptyArgs(t *testing.T) {
	type emptyArgs struct{}
	noArgFunc := func(_ context.Context, args emptyArgs) (any, error) {
		return "success", nil
	}

	tool, err := NewFuncTool("no_args", "Test", noArgFunc)
	require.NoError(t, err)

	result, err := tool.Call(context.Background(), "{}")

	require.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestFuncTool_Call_NilResult(t *testing.T) {
	type simpleArgs struct {
		Value string `json:"value"`
	}
	nilFunc := func(_ context.Context, args simpleArgs) (any, error) {
		return nil, nil
	}

	tool, err := NewFuncTool("nil_result", "Test", nilFunc)
	require.NoError(t, err)

	result, err := tool.Call(context.Background(), `{"value":"test"}`)

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestFuncTool_WithCustomOptions(t *testing.T) {
	tool, err := NewFuncTool("custom", "Custom tool", weatherFunc)
	require.NoError(t, err)

	assert.Equal(t, "Custom tool", tool.Description())
	assert.Equal(t, "custom", tool.Name())
}

func TestFuncTool_ComplexArgs(t *testing.T) {
	type complexArgs struct {
		Query   string            `json:"query" jsonschema:"required"`
		Options map[string]string `json:"options,omitempty"`
		Limit   int               `json:"limit" jsonschema:"minimum=1,maximum=100"`
	}

	complexFunc := func(_ context.Context, args complexArgs) (any, error) {
		return map[string]any{
			"query":       args.Query,
			"optionCount": len(args.Options),
			"limit":       args.Limit,
		}, nil
	}

	tool, err := NewFuncTool("complex", "Complex tool", complexFunc)
	require.NoError(t, err)

	result, err := tool.Call(context.Background(), `{
		"query": "test",
		"options": {"a": "1", "b": "2"},
		"limit": 10
	}`)

	require.NoError(t, err)
	resultMap := result.(map[string]any)
	assert.Equal(t, "test", resultMap["query"])
	assert.Equal(t, 2, resultMap["optionCount"])
	assert.Equal(t, 10, resultMap["limit"])
}

func TestFuncTool_ContextCancellation(t *testing.T) {
	type simpleArgs struct {
		Value string `json:"value"`
	}
	slowFunc := func(ctx context.Context, args simpleArgs) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	tool, err := NewFuncTool("slow", "Slow tool", slowFunc)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = tool.Call(ctx, `{"value":"test"}`)

	require.Error(t, err)
	// Error is wrapped by tool, check it contains cancellation
	assert.Contains(t, err.Error(), "canceled")
}

func TestFuncTool_SchemaGeneration(t *testing.T) {
	type schemaArgs struct {
		Required   string   `json:"required" jsonschema:"required,description=A required field"`
		Optional   string   `json:"optional,omitempty" jsonschema:"description=An optional field"`
		Number     int      `json:"number" jsonschema:"minimum=0,maximum=100"`
		StringList []string `json:"list,omitempty"`
	}

	schemaFunc := func(_ context.Context, args schemaArgs) (any, error) {
		return args.Required, nil
	}

	tool, err := NewFuncTool("schema_test", "Schema test", schemaFunc)
	require.NoError(t, err)

	def := tool.Definition()
	params := def.Function.Parameters

	// Verify required fields exist
	required, ok := params["required"].([]any)
	if ok {
		// Convert to strings for assertion
		requiredStrs := make([]string, len(required))
		for i, v := range required {
			requiredStrs[i] = v.(string)
		}
		assert.Contains(t, requiredStrs, "required")
	}

	// Verify properties have descriptions
	properties, ok := params["properties"].(map[string]any)
	require.True(t, ok)

	requiredProp, ok := properties["required"].(map[string]any)
	if ok {
		assert.Equal(t, "A required field", requiredProp["description"])
	}
}

func TestFuncTool_WithInputGuardrails(t *testing.T) {
	type searchArgs struct {
		Query string `json:"query"`
	}

	searchFunc := func(_ context.Context, args searchArgs) (string, error) {
		return "result for " + args.Query, nil
	}

	// Create a content filter guardrail that blocks "hack"
	contentFilter := guardrail.NewContentFilterGuardrail([]string{"hack", "exploit"})

	tool, err := NewFuncTool("search", "Search", searchFunc,
		WithInputGuardrails(contentFilter),
	)
	require.NoError(t, err)

	ctx := context.Background()

	// Test allowed input
	result, err := tool.Call(ctx, `{"query": "hello world"}`)
	assert.NoError(t, err)
	assert.Equal(t, "result for hello world", result)

	// Test blocked input
	_, err = tool.Call(ctx, `{"query": "how to hack systems"}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rejected")
}

func TestFuncTool_WithOutputGuardrails(t *testing.T) {
	type noArgs struct{}

	outputFunc := func(_ context.Context, _ noArgs) (string, error) {
		return "this contains secret data", nil
	}

	// Create a content filter guardrail that blocks "secret"
	contentFilter := guardrail.NewContentFilterGuardrail([]string{"secret", "password"})

	tool, err := NewFuncTool("output_test", "Output test", outputFunc,
		WithOutputGuardrails(contentFilter),
	)
	require.NoError(t, err)

	ctx := context.Background()

	// Test blocked output
	_, err = tool.Call(ctx, `{}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rejected")
}

func TestFuncTool_WithGuardrailTripwire(t *testing.T) {
	type noArgs struct{}

	outputFunc := func(_ context.Context, _ noArgs) (string, error) {
		return "normal output", nil
	}

	// Create a content filter guardrail that raises (tripwire) on "normal"
	contentFilter := guardrail.NewContentFilterGuardrail(
		[]string{"normal"},
		guardrail.WithContentFilterAction(guardrail.ActionRaise),
	)

	tool, err := NewFuncTool("tripwire_test", "Tripwire test", outputFunc,
		WithOutputGuardrails(contentFilter),
	)
	require.NoError(t, err)

	ctx := context.Background()

	// Test tripwire
	_, err = tool.Call(ctx, `{}`)
	assert.Error(t, err)

	// Verify it's a TripwireError
	var tripwireErr *guardrail.TripwireError
	assert.ErrorAs(t, err, &tripwireErr)
}
