package tool

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sampleSchema struct {
	A string `json:"a" description:"Field A"`
	B *int   `json:"b" description:"Optional pointer field"`
	C int    `json:"c,omitempty" description:"Omit empty field"`
}

func TestInferParameterSchema(t *testing.T) {
	schema, err := InferParameterSchema(sampleSchema{})
	require.NoError(t, err)

	props, ok := schema["properties"].(map[string]any)
	assert.True(t, ok)
	// Properties present
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")
	assert.Contains(t, props, "c")
	// Required includes fields without 'omitempty' (jsonschema-go behavior)
	var req []string
	if r, ok := schema["required"].([]string); ok {
		req = r
	} else if rAny, ok := schema["required"].([]any); ok {
		for _, v := range rAny {
			if s, ok := v.(string); ok {
				req = append(req, s)
			}
		}
	}
	// a should be required
	assert.Contains(t, req, "a")
	// c has omitempty and should not be required
	assert.NotContains(t, req, "c")
}

func TestInferParameterSchema_VariousInputs(t *testing.T) {
	inputs := []any{sampleSchema{}, &sampleSchema{}, reflect.TypeOf(sampleSchema{})}
	for _, in := range inputs {
		schema, err := InferParameterSchema(in)
		require.NoError(t, err)
		props, ok := schema["properties"].(map[string]any)
		assert.True(t, ok)
		assert.Contains(t, props, "a")
	}
}

func TestValidateParameters(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
		// Use []any to mirror possible JSON decoded schema shape
		"required": []any{"x"},
	}

	// Success
	err := ValidateParameters(schema, map[string]any{"x": 5})
	assert.NoError(t, err)

	// Wrong type
	err = ValidateParameters(schema, map[string]any{"x": "not-int"})
	assert.Error(t, err)

	// Missing required
	err = ValidateParameters(schema, map[string]any{})
	assert.Error(t, err)
}

func TestErrorFormatting(t *testing.T) {
	err := NewError("demo", "something failed", "E123")
	assert.Contains(t, err.Error(), "E123")
	assert.Contains(t, err.Error(), "demo")
}
