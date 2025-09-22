package tool

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hupe1980/agentmesh/core"
)

// Error represents errors that occur during tool execution.
type Error struct {
	Tool    string `json:"tool"`              // Name of the tool that failed
	Message string `json:"message"`           // Error message
	Code    string `json:"code"`              // Error code for categorization
	Details any    `json:"details,omitempty"` // Additional error details
}

// NewError creates a new Error with the specified details.
func NewError(tool, message, code string) *Error {
	return &Error{
		Tool:    tool,
		Message: message,
		Code:    code,
	}
}

// Error implements the error interface for Tool errors.
func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("tool error [%s] in %s: %s", e.Code, e.Tool, e.Message)
	}

	return fmt.Sprintf("tool error in %s: %s", e.Tool, e.Message)
}

// InferParameterSchema infers a JSON Schema (as map[string]any) for the given struct type
// using jsonschema-go. structType may be:
//   - a struct value (e.g., SumArgs{}),
//   - a pointer to struct (e.g., &SumArgs{}), or
//   - a reflect.Type describing the struct type (e.g., reflect.TypeOf(SumArgs{})).
func InferParameterSchema(structType any) (map[string]any, error) {
	var t reflect.Type

	if rt, ok := structType.(reflect.Type); ok {
		t = rt
	} else {
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

	s, err := jsonschema.ForType(t, &jsonschema.ForOptions{IgnoreInvalidTypes: true})
	if err != nil {
		return nil, fmt.Errorf("jsonschema inference failed: %w", err)
	}

	if s == nil {
		return nil, fmt.Errorf("jsonschema returned nil schema")
	}

	schema, err := core.SchemaToMap(s)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	if schema == nil {
		return nil, fmt.Errorf("converted schema is nil")
	}

	return schema, nil
}

// ValidateParameters validates the provided args against the given JSON Schema-like
// parameters map using jsonschema-go. It returns a descriptive error on failure.
func ValidateParameters(parameters map[string]any, args map[string]any) error {
	var schema jsonschema.Schema
	b, err := json.Marshal(parameters)
	if err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	if err := json.Unmarshal(b, &schema); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("schema resolve error: %w", err)
	}

	if err := resolved.Validate(args); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}

	return nil
}
