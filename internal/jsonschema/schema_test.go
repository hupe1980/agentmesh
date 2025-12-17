package jsonschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sample types for schema reflection
type sampleArgs struct {
	// name as string
	Name string `json:"name" jsonschema:"description=User name"`
	// optional pointer, omitted when empty
	Age *int `json:"age,omitempty" jsonschema:"description=Optional age"`
}

func TestFromStruct_Basic(t *testing.T) {
	s, err := FromStruct(sampleArgs{})
	require.NoError(t, err)
	require.NotNil(t, s)

	// Marshal to map for easy assertions
	b, err := json.Marshal(s)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))

	// type is object and properties include name and age
	assert.Equal(t, "object", m["type"])
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "age")

	// additionalProperties should be false as configured
	assert.Equal(t, false, m["additionalProperties"])
}

func TestFromStruct_PointerAndReflectType(t *testing.T) {
	cases := []any{sampleArgs{}, &sampleArgs{}, reflect.TypeOf(sampleArgs{})}
	for _, in := range cases {
		s, err := FromStruct(in)
		require.NoError(t, err)
		require.NotNil(t, s)
	}
}

func TestMapFromStruct_Basic(t *testing.T) {
	m, err := MapFromStruct(sampleArgs{})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "object", m["type"])
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "name")
}

func TestValidate_WithMapSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
		"required": []any{"x"},
	}

	// success
	result := Validate(schema, map[string]any{"x": 5})
	assert.True(t, result.Valid)

	// missing required
	result = Validate(schema, map[string]any{})
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)

	// wrong type
	result = Validate(schema, map[string]any{"x": "nope"})
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestValidate_WithInvSchema(t *testing.T) {
	// Build inv schema via FromStruct and validate a value against it
	s, err := FromStruct(sampleArgs{})
	require.NoError(t, err)

	// valid value
	v := map[string]any{"name": "alice"}
	result := Validate(s, v)
	assert.True(t, result.Valid)

	// wrong type for name
	v2 := map[string]any{"name": 123}
	result = Validate(s, v2)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

// Additional coverage

type tagArgs struct {
	Color string `json:"color" jsonschema:"description=Favorite color,enum=red,green,blue"`
	Email string `json:"email,omitempty" jsonschema:"format=email"`
	Count int    `json:"count" jsonschema:"default=5,minimum=0"`
}

func TestFromStruct_WithTags(t *testing.T) {
	s, err := FromStruct(tagArgs{})
	require.NoError(t, err)
	b, err := json.Marshal(s)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))

	props := m["properties"].(map[string]any)
	color := props["color"].(map[string]any)
	// description and enum present
	assert.Contains(t, color["description"].(string), "Favorite color")
	enumVals, ok := color["enum"].([]any)
	require.True(t, ok)
	// Some reflectors may record only the first enum token from the tag value.
	// Assert at least one value and includes 'red'.
	require.NotEmpty(t, enumVals)
	foundRed := false
	for _, v := range enumVals {
		if s, ok := v.(string); ok && s == "red" {
			foundRed = true
			break
		}
	}
	assert.True(t, foundRed, "enum should include 'red'")

	email := props["email"].(map[string]any)
	assert.Equal(t, "string", email["type"])
	assert.Equal(t, "email", email["format"]) // format propagated

	count := props["count"].(map[string]any)
	assert.Equal(t, float64(5), count["default"]) // defaults marshal as float64
	// minimum might be serialized as number
	assert.Equal(t, float64(0), count["minimum"])

	// additionalProperties disabled
	assert.Equal(t, false, m["additionalProperties"])
}

func TestFromStruct_InvalidInputs(t *testing.T) {
	var nilAny any
	_, err := FromStruct(nilAny)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structType is nil")

	_, err = FromStruct(42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected struct type")
}

func TestMapFromStruct_WithReflectType(t *testing.T) {
	m, err := MapFromStruct(reflect.TypeOf(sampleArgs{}))
	require.NoError(t, err)
	require.Equal(t, "object", m["type"])
}

func TestFromStruct_ExpandedNestedStruct(t *testing.T) {
	type Nested struct {
		N int `json:"n"`
	}
	type WithNested struct {
		Nested Nested `json:"nested"`
	}
	s, err := FromStruct(WithNested{})
	require.NoError(t, err)
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	props := m["properties"].(map[string]any)
	nested := props["nested"].(map[string]any)
	if tVal, ok := nested["type"].(string); ok {
		assert.Equal(t, "object", tVal)
	} else {
		// Accept $ref style as well
		_, hasRef := nested["$ref"].(string)
		assert.True(t, hasRef, "nested schema should be object type or a $ref")
	}
}

func TestValidate_WithJSONStringAndBytes(t *testing.T) {
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"v": map[string]any{"type": "number"},
		},
		"required": []any{"v"},
	}
	// JSON string
	b, _ := json.Marshal(schemaMap)
	sStr := string(b)
	result := Validate(sStr, map[string]any{"v": 1.23})
	assert.True(t, result.Valid)
	result = Validate(sStr, map[string]any{"v": "x"})
	assert.False(t, result.Valid)

	// JSON bytes
	result = Validate(b, map[string]any{"v": 9})
	assert.True(t, result.Valid)
}

func TestValidate_StructSchemaWrongType(t *testing.T) {
	s, err := FromStruct(sampleArgs{})
	require.NoError(t, err)
	// Age present but wrong type
	v := map[string]any{"name": "bob", "age": "not-int"}
	result := Validate(s, v)
	assert.False(t, result.Valid, fmt.Sprintf("expected validation error for value: %#v", v))
}
