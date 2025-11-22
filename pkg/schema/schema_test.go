package schema

import (
	"encoding/json"
	"testing"
)

func TestNewOutputSchema_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name" jsonschema:"required,description=Person's name"`
		Age  int    `json:"age" jsonschema:"required,minimum=0,maximum=150"`
	}

	output, err := NewOutputSchema("person", Person{})
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	if output.Name != "person" {
		t.Errorf("Expected name 'person', got '%s'", output.Name)
	}

	if !output.Strict {
		t.Error("Expected Strict to be true by default")
	}

	if output.Schema == nil {
		t.Fatal("Schema should not be nil")
	}

	// Verify schema has required fields
	if _, ok := output.Schema["type"]; !ok {
		t.Error("Schema missing 'type' field")
	}
	if _, ok := output.Schema["properties"]; !ok {
		t.Error("Schema missing 'properties' field")
	}
	if _, ok := output.Schema["required"]; !ok {
		t.Error("Schema missing 'required' field")
	}
}

func TestNewOutputSchema_Map(t *testing.T) {
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []string{"name"},
	}

	output, err := NewOutputSchema("test", schemaMap)
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	if output.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", output.Name)
	}
}

func TestNewOutputSchema_WithOptions(t *testing.T) {
	type Simple struct {
		Value string `json:"value" jsonschema:"required"`
	}

	output, err := NewOutputSchema("test", Simple{},
		WithStrict(false),
		WithDescription("Test schema"),
		WithAllowAdditionalProperties(true),
	)
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	if output.Strict {
		t.Error("Expected Strict to be false")
	}

	if output.Description != "Test schema" {
		t.Errorf("Expected description 'Test schema', got '%s'", output.Description)
	}

	if allow, ok := output.Schema["additionalProperties"].(bool); !ok || !allow {
		t.Error("Expected additionalProperties to be true")
	}
}

func TestValidate(t *testing.T) {
	type Person struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	output, err := NewOutputSchema("person", Person{})
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	// Valid value
	validValue := map[string]any{
		"name": "John Doe",
		"age":  30,
	}

	if err := Validate(output.Schema, validValue); err != nil {
		t.Errorf("Validation should succeed for valid value: %v", err)
	}

	// Test with JSON string
	validJSON, _ := json.Marshal(validValue)
	if err := Validate(output.Schema, string(validJSON)); err != nil {
		t.Errorf("Validation should succeed for valid JSON string: %v", err)
	}
}
