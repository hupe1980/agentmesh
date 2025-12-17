package jsonschema

import (
	"encoding/json"
	"testing"

	stjs "github.com/santhosh-tekuri/jsonschema/v6"
)

// Test schema - moderate complexity
var benchSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"name":   map[string]any{"type": "string", "minLength": 1, "maxLength": 100},
		"age":    map[string]any{"type": "integer", "minimum": 0, "maximum": 150},
		"email":  map[string]any{"type": "string", "format": "email"},
		"status": map[string]any{"type": "string", "enum": []any{"active", "inactive", "pending"}},
		"tags": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"address": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"street":  map[string]any{"type": "string"},
				"city":    map[string]any{"type": "string"},
				"zipcode": map[string]any{"type": "string"},
			},
			"required": []any{"street", "city"},
		},
	},
	"required":             []any{"name", "age", "status"},
	"additionalProperties": false,
}

// Valid JSON for benchmarks
var validJSON = `{
	"name": "John Doe",
	"age": 30,
	"email": "john@example.com",
	"status": "active",
	"tags": ["developer", "golang"],
	"address": {
		"street": "123 Main St",
		"city": "New York",
		"zipcode": "10001"
	}
}`

// Invalid JSON with multiple errors
var invalidJSON = `{
	"name": 123,
	"age": "thirty",
	"status": "unknown",
	"extra": "field"
}`

// ============================================================================
// Santhosh-Tekuri jsonschema benchmarks (now the default)
// ============================================================================

func setupSanthoshSchema(b *testing.B) *stjs.Schema {
	b.Helper()
	c := stjs.NewCompiler()
	if err := c.AddResource("schema.json", benchSchema); err != nil {
		b.Fatal(err)
	}
	schema, err := c.Compile("schema.json")
	if err != nil {
		b.Fatal(err)
	}
	return schema
}

func BenchmarkSanthosh_CompileSchema(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := stjs.NewCompiler()
		_ = c.AddResource("schema.json", benchSchema)
		_, _ = c.Compile("schema.json")
	}
}

func BenchmarkSanthosh_ValidateValid(b *testing.B) {
	schema := setupSanthoshSchema(b)
	var data any
	json.Unmarshal([]byte(validJSON), &data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = schema.Validate(data)
	}
}

func BenchmarkSanthosh_ValidateInvalid(b *testing.B) {
	schema := setupSanthoshSchema(b)
	var data any
	json.Unmarshal([]byte(invalidJSON), &data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = schema.Validate(data)
	}
}

func BenchmarkSanthosh_EndToEnd_Valid(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := stjs.NewCompiler()
		_ = c.AddResource("schema.json", benchSchema)
		schema, _ := c.Compile("schema.json")
		var data any
		json.Unmarshal([]byte(validJSON), &data)
		_ = schema.Validate(data)
	}
}

func BenchmarkSanthosh_EndToEnd_Invalid(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := stjs.NewCompiler()
		_ = c.AddResource("schema.json", benchSchema)
		schema, _ := c.Compile("schema.json")
		var data any
		json.Unmarshal([]byte(invalidJSON), &data)
		_ = schema.Validate(data)
	}
}

// ============================================================================
// Comparison: Error collection overhead (uses the package's collectValidationErrors)
// ============================================================================

func BenchmarkSanthosh_CollectAllErrors(b *testing.B) {
	schema := setupSanthoshSchema(b)
	var data any
	json.Unmarshal([]byte(invalidJSON), &data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := schema.Validate(data)
		if ve, ok := err.(*stjs.ValidationError); ok {
			_ = collectValidationErrors(ve)
		}
	}
}
