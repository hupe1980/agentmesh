package jsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	inv "github.com/invopop/jsonschema"
	stjs "github.com/santhosh-tekuri/jsonschema/v6"
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

// ValidationError represents a single validation error with path and message.
type ValidationError struct {
	Path    string // JSON path to the error (e.g., "/properties/age")
	Message string // Human-readable error message
}

// ValidationResult contains all validation errors.
type ValidationResult struct {
	Valid      bool              // True if validation passed
	Errors     []ValidationError // All validation errors found
	ParsedData any               // The parsed JSON data (if valid JSON)
}

// Validate checks value against a JSON Schema and returns all validation errors.
// Supported schema types:
//   - map[string]any (e.g., from MapFromStruct)
//   - *inv.Schema (e.g., from FromStruct)
//   - []byte or string containing JSON Schema
//
// Value can be:
//   - Go types (map[string]any, struct, etc.)
//   - JSON string ([]byte or string)
//
//nolint:gocyclo // Complex schema validation logic; refactoring would reduce readability
func Validate(schema any, value any) *ValidationResult {
	// Normalize schema to map[string]any
	var schemaMap map[string]any
	switch s := schema.(type) {
	case map[string]any:
		schemaMap = s
	case *inv.Schema:
		b, err := json.Marshal(s)
		if err != nil {
			return &ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("marshal schema: %v", err)}},
			}
		}
		if err := json.Unmarshal(b, &schemaMap); err != nil {
			return &ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("unmarshal schema: %v", err)}},
			}
		}
	case []byte:
		if err := json.Unmarshal(s, &schemaMap); err != nil {
			return &ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("invalid schema json: %v", err)}},
			}
		}
	case string:
		if err := json.Unmarshal([]byte(s), &schemaMap); err != nil {
			return &ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("invalid schema json: %v", err)}},
			}
		}
	default:
		return &ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("unsupported schema type %T", schema)}},
		}
	}

	// Compile schema using santhosh-tekuri
	c := stjs.NewCompiler()
	if err := c.AddResource("schema.json", schemaMap); err != nil {
		return &ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("add schema resource: %v", err)}},
		}
	}

	compiled, err := c.Compile("schema.json")
	if err != nil {
		return &ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Path: "", Message: fmt.Sprintf("compile schema: %v", err)}},
		}
	}

	// Normalize value
	var val any
	switch v := value.(type) {
	case string:
		if v == "" {
			return &ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Path: "", Message: "empty string"}},
			}
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

	// Validate
	if err := compiled.Validate(val); err != nil {
		var ve *stjs.ValidationError
		if errors.As(err, &ve) {
			errs := collectValidationErrors(ve)
			return &ValidationResult{
				Valid:      false,
				Errors:     errs,
				ParsedData: val,
			}
		}
		return &ValidationResult{
			Valid:      false,
			Errors:     []ValidationError{{Path: "", Message: err.Error()}},
			ParsedData: val,
		}
	}

	return &ValidationResult{Valid: true, ParsedData: val}
}

// collectValidationErrors recursively collects all validation errors from the error tree.
func collectValidationErrors(ve *stjs.ValidationError) []ValidationError {
	var errors []ValidationError
	collectErrorsRecursive(ve, &errors)
	return errors
}

func collectErrorsRecursive(ve *stjs.ValidationError, errors *[]ValidationError) {
	// Leaf errors have no causes - these are the actual validation failures
	if len(ve.Causes) == 0 {
		// Convert InstanceLocation ([]string) to JSON pointer path
		path := "/" + strings.Join(ve.InstanceLocation, "/")
		if path == "/" {
			path = ""
		}
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: ve.Error(),
		})
		return
	}
	// Recurse into causes
	for _, cause := range ve.Causes {
		collectErrorsRecursive(cause, errors)
	}
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
