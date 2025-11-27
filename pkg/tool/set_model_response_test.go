package tool

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSetModelResponseTool_FromStruct(t *testing.T) {
	type TestOutput struct {
		Result string `json:"result" jsonschema:"required,description=The result value"`
		Count  int    `json:"count" jsonschema:"required,description=The count value"`
	}

	tool, err := NewSetModelResponseTool(TestOutput{})
	require.NoError(t, err)

	assert.Equal(t, "set_model_response", tool.Name())
	assert.Contains(t, tool.Description(), "final response")

	params := tool.Parameters()
	assert.Equal(t, "object", params["type"])
	assert.NotNil(t, params["properties"])
	assert.NotNil(t, params["required"])
}

func TestNewSetModelResponseTool_FromMap(t *testing.T) {
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The message",
			},
		},
		"required": []any{"message"},
	}

	tool, err := NewSetModelResponseTool(schemaMap)
	require.NoError(t, err)

	assert.Equal(t, "set_model_response", tool.Name())

	params := tool.Parameters()
	props := params["properties"].(map[string]any)
	assert.Contains(t, props, "message")
}

func TestNewSetModelResponseTool_FromOutputSchema(t *testing.T) {
	type TestOutput struct {
		Answer string `json:"answer" jsonschema:"required,description=The answer"`
	}

	outputSchema, err := schema.NewOutputSchema[TestOutput]("test_output", TestOutput{})
	require.NoError(t, err)

	tool, err := NewSetModelResponseTool(outputSchema)
	require.NoError(t, err)

	assert.Equal(t, "set_model_response", tool.Name())

	params := tool.Parameters()
	props := params["properties"].(map[string]any)
	assert.Contains(t, props, "answer")
}

func TestNewSetModelResponseTool_MinimalSchema(t *testing.T) {
	// Schema with minimal fields is accepted
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"field": map[string]any{
				"type": "string",
			},
		},
		"required": []any{"field"},
	}

	tool, err := NewSetModelResponseTool(schemaMap)
	require.NoError(t, err)
	assert.NotNil(t, tool)
}

func TestNewSetModelResponseTool_InvalidType(t *testing.T) {
	_, err := NewSetModelResponseTool("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected struct type")
}

func TestSetModelResponseTool_Definition(t *testing.T) {
	type TestOutput struct {
		Value string `json:"value" jsonschema:"required,description=The value"`
	}

	tool, err := NewSetModelResponseTool(TestOutput{})
	require.NoError(t, err)

	def := tool.Definition()
	require.NotNil(t, def)

	assert.Equal(t, "function", def.Type)
	assert.Equal(t, "set_model_response", def.Function.Name)
	assert.NotEmpty(t, def.Function.Description)
	assert.Equal(t, "object", def.Function.Parameters["type"])
	assert.NotNil(t, def.Function.Parameters["properties"])
	assert.NotNil(t, def.Function.Parameters["required"])
}

func TestSetModelResponseTool_Call(t *testing.T) {
	type TestOutput struct {
		Message  string `json:"message" jsonschema:"required,description=The message"`
		Status   string `json:"status" jsonschema:"required,description=The status"`
		Optional string `json:"optional,omitempty" jsonschema:"description=Optional field"`
	}

	tool, err := NewSetModelResponseTool(TestOutput{})
	require.NoError(t, err)

	tests := []struct {
		name        string
		args        string
		wantErr     bool
		errContains string
		wantResult  string
	}{
		{
			name:       "valid",
			args:       `{"message": "Hello", "status": "success"}`,
			wantErr:    false,
			wantResult: `{"message": "Hello", "status": "success"}`,
		},
		{
			name:        "invalid_missing_required",
			args:        `{"optional": "value"}`,
			wantErr:     true,
			errContains: "invalid model response",
		},
		{
			name:    "invalid_json",
			args:    `{invalid json}`,
			wantErr: true,
		},
		{
			name:       "valid_with_optional",
			args:       `{"message": "Hi", "status": "ok", "optional": "extra"}`,
			wantErr:    false,
			wantResult: `{"message": "Hi", "status": "ok", "optional": "extra"}`,
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
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestSetModelResponseTool_Instruction(t *testing.T) {
	type TestOutput struct {
		Result string `json:"result" jsonschema:"required"`
	}

	tool, err := NewSetModelResponseTool(TestOutput{})
	require.NoError(t, err)

	instruction := tool.Instruction()
	assert.NotEmpty(t, instruction)
	assert.Contains(t, instruction, "set_model_response")
	assert.Contains(t, instruction, "final response")
	assert.Contains(t, instruction, "IMPORTANT")
}

func TestSetModelResponseTool_Parameters(t *testing.T) {
	type ComplexOutput struct {
		Name     string         `json:"name" jsonschema:"required,description=Person name"`
		Age      int            `json:"age" jsonschema:"required,description=Person age"`
		Hobbies  []string       `json:"hobbies" jsonschema:"description=List of hobbies"`
		Active   bool           `json:"active" jsonschema:"required,description=Is active"`
		Metadata map[string]any `json:"metadata" jsonschema:"description=Additional metadata"`
	}

	tool, err := NewSetModelResponseTool(ComplexOutput{})
	require.NoError(t, err)

	params := tool.Parameters()
	assert.Equal(t, "object", params["type"])

	props := params["properties"].(map[string]any)
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "age")
	assert.Contains(t, props, "hobbies")
	assert.Contains(t, props, "active")
	assert.Contains(t, props, "metadata")

	required := params["required"].([]any)
	// All fields with required tag should be in required list
	assert.Contains(t, required, "name")
	assert.Contains(t, required, "age")
	assert.Contains(t, required, "active")
	// Fields without required tag may still be in required - jsonschema behavior
	// Just verify the explicitly required ones are present
}

func TestSetModelResponseTool_ImplementsToolInterface(t *testing.T) {
	type TestOutput struct {
		Field string `json:"field" jsonschema:"required"`
	}

	tool, err := NewSetModelResponseTool(TestOutput{})
	require.NoError(t, err)

	// Verify it implements the Tool interface
	var _ Tool = tool

	// All required methods should work
	assert.NotEmpty(t, tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Definition())

	result, err := tool.Call(context.Background(), `{"field": "value"}`)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}
