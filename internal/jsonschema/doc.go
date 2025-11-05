/*
Package jsonschema provides automatic JSON Schema generation from Go types.

This package is used internally by the tool package to generate JSON schemas
for function parameters, enabling automatic tool schema generation for LLM
function calling.

# Overview

The package uses reflection to introspect Go types and produce JSON Schema
definitions that describe:
  - Primitive types (string, int, float, bool)
  - Struct types with field tags
  - Slice and array types
  - Map types
  - Nested structures
  - Optional fields (omitempty)

# Field Tags

Struct field tags control schema generation:

	type Args struct {
		Name     string   `json:"name" description:"User name"`
		Age      int      `json:"age,omitempty" description:"User age"`
		Tags     []string `json:"tags" description:"User tags"`
	}

Generated schema:

	{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "User name"},
			"age": {"type": "integer", "description": "User age"},
			"tags": {"type": "array", "items": {"type": "string"}, "description": "User tags"}
		},
		"required": ["name", "tags"]
	}

# Usage

Generate schema from a type:

	schema, err := jsonschema.FromType(reflect.TypeOf(Args{}))
	// Returns map[string]any representing JSON Schema
*/
package jsonschema
