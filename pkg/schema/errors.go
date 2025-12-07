// Package schema provides sentinel errors for the schema package.
package schema

import "errors"

var (
	// ErrMissingType is returned when output schema is missing 'type'.
	ErrMissingType = errors.New("schema: output schema missing 'type'")

	// ErrMissingProperties is returned when output schema is missing 'properties'.
	ErrMissingProperties = errors.New("schema: output schema missing 'properties'")

	// ErrMissingRequired is returned when output schema is missing 'required'.
	ErrMissingRequired = errors.New("schema: output schema missing 'required'")

	// ErrNoMessages is returned when no messages are returned.
	ErrNoMessages = errors.New("schema: no messages returned")
)
