package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_InvalidJSON(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
		"required":   []any{"name"},
	}

	result, err := validator.Validate(context.Background(), schema, "not json")
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Message, "invalid JSON")
}

func TestValidator_ValidObject(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "number"},
		},
		"required": []any{"name"},
	}

	result, err := validator.Validate(context.Background(), schema, `{"name": "John", "age": 30}`)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	assert.NotNil(t, result.ParsedData)
}

func TestValidator_MissingRequired(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "number"},
		},
		"required": []any{"name", "age"},
	}

	result, err := validator.Validate(context.Background(), schema, `{"name": "John"}`)
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestValidator_TypeMismatch(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"age": map[string]any{"type": "number"},
		},
		"required": []any{"age"},
	}

	result, err := validator.Validate(context.Background(), schema, `{"age": "thirty"}`)
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestValidator_NestedObject(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"person": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			},
		},
		"required": []any{"person"},
	}

	t.Run("valid nested", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"person": {"name": "John"}}`)
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("missing nested required", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"person": {}}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})
}

func TestValidator_Array(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "number"},
					},
					"required": []any{"id"},
				},
			},
		},
		"required": []any{"items"},
	}

	t.Run("valid array", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"items": [{"id": 1}, {"id": 2}]}`)
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("invalid array item", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"items": [{"id": 1}, {}]}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})
}

func TestValidator_Enum(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []any{"active", "inactive", "pending"},
			},
		},
		"required": []any{"status"},
	}

	t.Run("valid enum", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"status": "active"}`)
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("invalid enum", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"status": "unknown"}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})
}

func TestValidator_StringConstraints(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"minLength": float64(3),
				"maxLength": float64(10),
			},
		},
		"required": []any{"name"},
	}

	t.Run("valid length", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"name": "John"}`)
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("too short", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"name": "Jo"}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})

	t.Run("too long", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"name": "JohnJohnJohn"}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})
}

func TestValidator_NumberConstraints(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"age": map[string]any{
				"type":    "number",
				"minimum": float64(0),
				"maximum": float64(150),
			},
		},
		"required": []any{"age"},
	}

	t.Run("valid range", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"age": 30}`)
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("below minimum", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"age": -5}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})

	t.Run("above maximum", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"age": 200}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})
}

func TestValidator_AdditionalProperties(t *testing.T) {
	validator := NewValidator()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required":             []any{"name"},
		"additionalProperties": false,
	}

	t.Run("no additional properties", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"name": "John"}`)
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("additional property rejected", func(t *testing.T) {
		result, err := validator.Validate(context.Background(), schema, `{"name": "John", "extra": "field"}`)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})
}

func TestValidator_TypeChecks(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name     string
		schema   map[string]any
		value    string
		expected bool
	}{
		{
			name:     "string type",
			schema:   map[string]any{"type": "string"},
			value:    `"hello"`,
			expected: true,
		},
		{
			name:     "number type",
			schema:   map[string]any{"type": "number"},
			value:    `42.5`,
			expected: true,
		},
		{
			name:     "integer type valid",
			schema:   map[string]any{"type": "integer"},
			value:    `42`,
			expected: true,
		},
		{
			name:     "integer type invalid (float)",
			schema:   map[string]any{"type": "integer"},
			value:    `42.5`,
			expected: false,
		},
		{
			name:     "boolean type",
			schema:   map[string]any{"type": "boolean"},
			value:    `true`,
			expected: true,
		},
		{
			name:     "array type",
			schema:   map[string]any{"type": "array"},
			value:    `[1, 2, 3]`,
			expected: true,
		},
		{
			name:     "object type",
			schema:   map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
			value:    `{}`,
			expected: true,
		},
		{
			name:     "null type",
			schema:   map[string]any{"type": "null"},
			value:    `null`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.Validate(context.Background(), tt.schema, tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Valid, "expected valid=%v for %s", tt.expected, tt.value)
		})
	}
}

// TestValidator_Interface ensures custom validators can be implemented.
func TestValidator_Interface(t *testing.T) {
	// Custom validator that always returns valid
	customValidator := &mockValidator{valid: true}

	schema := map[string]any{"type": "object"}
	result, err := customValidator.Validate(context.Background(), schema, `invalid`)
	require.NoError(t, err)
	assert.True(t, result.Valid)
}

type mockValidator struct {
	valid bool
}

func (m *mockValidator) Validate(_ context.Context, _ map[string]any, _ string) (*ValidationResult, error) {
	return &ValidationResult{Valid: m.valid}, nil
}
