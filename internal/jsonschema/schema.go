package jsonschema

import (
	"encoding/json"
	"fmt"
	"reflect"

	gjs "github.com/google/jsonschema-go/jsonschema"
	inv "github.com/invopop/jsonschema"
)

// FromStruct reflects a JSON Schema from a struct definition.
// The structType may be:
//   - a struct value (e.g., MyArgs{}),
//   - a pointer to struct (e.g., &MyArgs{}), or
//   - a reflect.Type (e.g., reflect.TypeOf(MyArgs{})).
//
// Tags honored:
//   - json: field names and "omitempty" (when RequiredFromJSONTags is true, fields
//     without omitempty are marked as required).
//   - jsonschema: constraints like title, description, enum, default, format, etc.
//     See https://github.com/invopop/jsonschema for full tag support.
func FromStruct(structType any) (*inv.Schema, error) {
	var t reflect.Type
	switch v := structType.(type) {
	case reflect.Type:
		t = v
	default:
		t = reflect.TypeOf(structType)
	}

	if t == nil {
		return nil, fmt.Errorf("structType is nil")
	}

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct type, got %s", t.Kind())
	}

	r := &inv.Reflector{
		// Expand all structs inline instead of $ref.
		// This is important for OpenAI function calling which doesn't support $ref.
		ExpandedStruct: true,
		// Disallow additionalProperties by default for stricter validation.
		AllowAdditionalProperties: false,
	}

	s := r.ReflectFromType(t)
	if s == nil {
		return nil, fmt.Errorf("jsonschema reflector returned nil schema")
	}

	return s, nil
}

// MapFromStruct generates a JSON Schema and returns it as a generic map.
// Useful if callers prefer a loosely-typed schema representation.
func MapFromStruct(structType any) (map[string]any, error) {
	s, err := FromStruct(structType)
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal schema map: %w", err)
	}

	return m, nil
}

// Validate checks value against a JSON Schema.
// Supported schema types:
//   - map[string]any (e.g., from MapFromStruct)
//   - *inv.Schema (e.g., from FromStruct)
//   - []byte or string containing JSON Schema
//
// Value can be:
//   - Go types (map[string]any, struct, etc.)
//   - JSON string ([]byte or string)
func Validate(schema any, value any) error {
	var raw []byte

	// Normalize schema into JSON
	switch s := schema.(type) {
	case map[string]any:
		b, err := json.Marshal(s)
		if err != nil {
			return fmt.Errorf("invalid schema map: %w", err)
		}
		raw = b
	case *inv.Schema:
		b, err := json.Marshal(s)
		if err != nil {
			return fmt.Errorf("marshal inv schema: %w", err)
		}
		raw = b
	case []byte:
		raw = s
	case string:
		raw = []byte(s)
	default:
		return fmt.Errorf("unsupported schema type %T", schema)
	}

	// Decode schema
	var gs gjs.Schema
	if err := json.Unmarshal(raw, &gs); err != nil {
		return fmt.Errorf("invalid schema json: %w", err)
	}

	resolved, err := gs.Resolve(nil)
	if err != nil {
		return fmt.Errorf("schema resolve error: %w", err)
	}

	// --- Normalize value ---
	var val any
	switch v := value.(type) {
	case string:
		if v == "" {
			return fmt.Errorf("invalid value: empty string")
		}
		if err := json.Unmarshal([]byte(v), &val); err != nil {
			// if not valid JSON, treat raw string as-is
			val = v
		}
	case []byte:
		if err := json.Unmarshal(v, &val); err != nil {
			val = string(v)
		}
	default:
		val = v
	}

	// Validate normalized value
	if err := resolved.Validate(val); err != nil {
		return fmt.Errorf("invalid value: %w", err)
	}

	return nil
}

// ToOpenAISchema generates a JSON Schema map compatible with OpenAI function calling.
func ToOpenAISchema(v any) (map[string]any, error) {
	r := &inv.Reflector{
		ExpandedStruct:            true,
		AllowAdditionalProperties: false,
	}

	schema := r.Reflect(v)

	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	clean := stripUnsupported(m)

	return clean, nil
}

// stripUnsupported removes fields not in OpenAI’s subset
func stripUnsupported(m map[string]any) map[string]any {
	allowed := map[string]bool{
		"type":        true,
		"properties":  true,
		"required":    true,
		"items":       true,
		"enum":        true,
		"description": true,
		"default":     true,
		"format":      true,
	}

	out := map[string]any{}
	for k, v := range m {
		if !allowed[k] {
			continue
		}

		switch val := v.(type) {
		case map[string]any:
			out[k] = stripUnsupported(val)
		case []any:
			newArr := make([]any, len(val))
			for i, elem := range val {
				if sub, ok := elem.(map[string]any); ok {
					newArr[i] = stripUnsupported(sub)
				} else {
					newArr[i] = elem
				}
			}
			out[k] = newArr
		default:
			out[k] = val
		}
	}

	return out
}
