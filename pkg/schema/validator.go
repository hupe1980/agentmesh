package schema

import (
	"context"
	"encoding/json"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

// Validator defines the interface for schema validation.
// Implementations can provide custom validation logic.
type Validator interface {
	// Validate checks if output conforms to the schema.
	Validate(ctx context.Context, schema map[string]any, output string) (*ValidationResult, error)
}

// ValidationResult contains the result of schema validation.
type ValidationResult struct {
	// Valid indicates if the output matches the schema.
	Valid bool

	// Errors contains validation errors.
	Errors []ValidationError

	// ParsedData is the parsed JSON data (if valid JSON).
	ParsedData any
}

// ValidationError represents a schema validation error.
type ValidationError struct {
	// Path is the JSON path to the error.
	Path string

	// Message describes the error.
	Message string

	// Expected is what the schema expected.
	Expected string

	// Actual is what was found.
	Actual string
}

// DefaultValidator validates output against a JSON schema.
// It implements the Validator interface using the internal jsonschema library.
type DefaultValidator struct{}

// Ensure DefaultValidator implements Validator.
var _ Validator = (*DefaultValidator)(nil)

// NewValidator creates a new default validator.
// This is the default Validator implementation using internal/jsonschema.
func NewValidator() *DefaultValidator {
	return &DefaultValidator{}
}

// Validate checks if output conforms to the schema using the jsonschema library.
func (v *DefaultValidator) Validate(_ context.Context, schema map[string]any, output string) (*ValidationResult, error) {
	// Parse JSON first
	var data any
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		//nolint:nilerr // Invalid JSON is a validation failure, not a system error
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{{
				Path:    "",
				Message: "invalid JSON: " + err.Error(),
			}},
		}, nil
	}

	// Validate using jsonschema library (pass parsed data to avoid double parsing)
	result := jsonschema.Validate(schema, data)
	if !result.Valid {
		// Convert internal errors to schema.ValidationError
		errors := make([]ValidationError, len(result.Errors))
		for i, e := range result.Errors {
			errors[i] = ValidationError{
				Path:    e.Path,
				Message: e.Message,
			}
		}
		return &ValidationResult{
			Valid:      false,
			Errors:     errors,
			ParsedData: data,
		}, nil
	}

	return &ValidationResult{
		Valid:      true,
		Errors:     nil,
		ParsedData: data,
	}, nil
}
