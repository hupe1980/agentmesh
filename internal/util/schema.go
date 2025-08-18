package util

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidationError represents parameter validation errors with detailed information.
type ValidationError struct {
	Field   string `json:"field"`   // Field that failed validation
	Value   any    `json:"value"`   // Value that was provided
	Message string `json:"message"` // Human-readable error message
}

// Error implements the error interface for ValidationError.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}

// CreateSchema creates a JSON schema from a Go struct using reflection.
// This is a convenience function for creating parameter schemas from Go types.
func CreateSchema(structType any) map[string]any {
	t := reflect.TypeOf(structType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	properties := make(map[string]any)
	required := make([]string, 0)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		fieldName := field.Name
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
		}

		fieldSchema := map[string]any{
			"type": getJSONType(field.Type),
		}

		if description := field.Tag.Get("description"); description != "" {
			fieldSchema["description"] = description
		}

		properties[fieldName] = fieldSchema

		if !hasOmitEmpty(field.Tag.Get("json")) && !isPointer(field.Type) {
			required = append(required, fieldName)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// ValidateParameters validates parameters against a JSON schema.
func ValidateParameters(params map[string]any, schema map[string]any) error {
	// Extract required fields
	required, _ := schema["required"].([]any)
	for _, req := range required {
		fieldName, ok := req.(string)
		if !ok {
			continue
		}
		if _, exists := params[fieldName]; !exists {
			return &ValidationError{
				Field:   fieldName,
				Message: "required field is missing",
			}
		}
	}

	// Validate field types
	properties, _ := schema["properties"].(map[string]any)
	for fieldName, value := range params {
		propSchema, exists := properties[fieldName]
		if !exists {
			continue // Allow extra fields
		}

		propMap, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}

		expectedType, _ := propMap["type"].(string)
		if !isValidType(value, expectedType) {
			return &ValidationError{
				Field:   fieldName,
				Value:   value,
				Message: fmt.Sprintf("expected type %s, got %T", expectedType, value),
			}
		}
	}

	return nil
}

// getJSONType returns the JSON schema type for a given Go type.
func getJSONType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Ptr:
		return getJSONType(t.Elem())
	default:
		return "string"
	}
}

// hasOmitEmpty checks if a JSON tag has the "omitempty" option.
func hasOmitEmpty(tag string) bool {
	parts := strings.Split(tag, ",")
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "omitempty" {
			return true
		}
	}
	return false
}

// isPointer checks if a type is a pointer.
func isPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr
}

// isValidType checks if a value is valid according to the expected JSON schema type.
func isValidType(value any, expectedType string) bool {
	if value == nil {
		return true // nil is valid for any type
	}

	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		switch v := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		case float64: // JSON unmarshaling often produces float64 for numbers
			return v == float64(int64(v)) // Check if it's actually an integer
		}
		return false
	case "number":
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
			float32, float64:
			return true
		}
		return false
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true // Unknown types are assumed valid
	}
}
