// Package schema provides utilities for creating and working with JSON schemas
// for structured output in AgentMesh agents and models.
package schema

import (
	"fmt"
	"maps"
	"reflect"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

// OutputSchema represents a structured output schema with metadata.
type OutputSchema struct {
	Name        string         // Schema name/identifier
	Strict      bool           // Enable strict mode (provider-specific)
	Description string         // Schema description
	Schema      map[string]any // The actual JSON schema
}

// OutputSchemaOptions configures OutputSchema creation.
type OutputSchemaOptions struct {
	Strict                    bool
	Description               string
	AllowAdditionalProperties bool
}

// WithStrict sets whether to enable strict mode for the schema.
// Strict mode behavior is provider-specific (e.g., OpenAI's strict JSON schema mode).
// Default: true
func WithStrict(strict bool) func(*OutputSchemaOptions) {
	return func(opts *OutputSchemaOptions) {
		opts.Strict = strict
	}
}

// WithDescription sets a description for the schema.
func WithDescription(description string) func(*OutputSchemaOptions) {
	return func(opts *OutputSchemaOptions) {
		opts.Description = description
	}
}

// WithAllowAdditionalProperties sets whether to allow additional properties
// not defined in the schema.
// Default: false
func WithAllowAdditionalProperties(allow bool) func(*OutputSchemaOptions) {
	return func(opts *OutputSchemaOptions) {
		opts.AllowAdditionalProperties = allow
	}
}

// NewOutputSchema creates an OutputSchema from a struct type or map[string]any.
// The function uses generics to accept either:
//   - A struct type (generates schema via reflection and jsonschema tags)
//   - A map[string]any (uses the map directly as the schema)
//
// Example with struct:
//
//	type Person struct {
//	    Name string `json:"name" jsonschema:"required,description=Person's name"`
//	    Age  int    `json:"age" jsonschema:"required,minimum=0,maximum=150"`
//	}
//	schema, err := schema.NewOutputSchema("person", Person{})
//
// Example with map:
//
//	schemaMap := map[string]any{
//	    "type": "object",
//	    "properties": map[string]any{
//	        "name": map[string]any{"type": "string"},
//	    },
//	    "required": []string{"name"},
//	}
//	schema, err := schema.NewOutputSchema("person", schemaMap)
//
// The function validates that the schema contains required fields:
// type, properties, and required.
func NewOutputSchema[T any](name string, schema T, optFns ...func(*OutputSchemaOptions)) (OutputSchema, error) {
	opts := OutputSchemaOptions{
		Strict:                    true,
		AllowAdditionalProperties: false,
	}

	for _, opt := range optFns {
		opt(&opts)
	}

	var finalSchema map[string]any
	val := reflect.ValueOf(schema)
	typ := val.Type()

	switch typ.Kind() {
	case reflect.Map:
		m, ok := any(schema).(map[string]any)
		if !ok {
			return OutputSchema{}, fmt.Errorf("expected map[string]any, got %T", schema)
		}

		finalSchema = maps.Clone(m)
	case reflect.Struct, reflect.Pointer:
		m, err := jsonschema.MapFromStruct(schema)
		if err != nil {
			return OutputSchema{}, fmt.Errorf("failed to convert struct to schema: %w", err)
		}

		finalSchema = m
	default:
		return OutputSchema{}, fmt.Errorf("unsupported schema type: %T", schema)
	}

	finalSchema["additionalProperties"] = opts.AllowAdditionalProperties

	// Validate minimal keys
	if _, ok := finalSchema["type"]; !ok {
		return OutputSchema{}, ErrMissingType
	}
	if _, ok := finalSchema["properties"]; !ok {
		return OutputSchema{}, ErrMissingProperties
	}
	if _, ok := finalSchema["required"]; !ok {
		return OutputSchema{}, ErrMissingRequired
	}

	return OutputSchema{
		Name:        name,
		Strict:      opts.Strict,
		Description: opts.Description,
		Schema:      finalSchema,
	}, nil
}

// Validate validates a value against a JSON schema.
// The value can be a map[string]any, struct, or JSON string.
//
// Example:
//
//	schema, _ := schema.NewOutputSchema("person", Person{})
//
//	value := map[string]any{
//	    "name": "John Doe",
//	    "age": 30,
//	}
//
//	if err := schema.Validate(schema.Schema, value); err != nil {
//	    fmt.Printf("Validation error: %v\n", err)
//	}
func Validate(schemaMap map[string]any, value any) error {
	return jsonschema.Validate(schemaMap, value)
}
