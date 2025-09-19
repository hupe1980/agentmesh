package core

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaToMap_Nil(t *testing.T) {
	got, err := SchemaToMap(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSchemaToMap_Empty(t *testing.T) {
	s := &jsonschema.Schema{}

	got, err := SchemaToMap(s)
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestSchemaToMap_ObjectWithProperties(t *testing.T) {
	// Define a simple object schema with one string property and a required field.
	schemaJSON := []byte(`{
        "title": "User",
        "type": "object",
        "properties": {
            "name": { "type": "string", "description": "User name" },
            "age": { "type": "integer" }
        },
        "required": ["name"]
    }`)

	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(schemaJSON, &s))

	got, err := SchemaToMap(&s)
	require.NoError(t, err)

	var want map[string]any
	require.NoError(t, json.Unmarshal(schemaJSON, &want))

	assert.Equal(t, want, got)
}

func TestSchemaToMap_ArrayItems(t *testing.T) {
	// Array schema with string items
	schemaJSON := []byte(`{
        "type": "array",
        "items": { "type": "string" },
        "minItems": 1
    }`)

	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(schemaJSON, &s))

	got, err := SchemaToMap(&s)
	require.NoError(t, err)

	var want map[string]any
	require.NoError(t, json.Unmarshal(schemaJSON, &want))

	assert.Equal(t, want, got)
}
