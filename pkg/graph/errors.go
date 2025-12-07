// Package graph provides structured errors for the graph package.
package graph

import (
	"fmt"
	"strings"
)

// =============================================================================
// Structured Errors
// =============================================================================

// ManagedValueError represents errors related to managed values.
// Use errors.As to extract the MissingValues field for programmatic handling.
type ManagedValueError struct {
	// MissingValues contains the names of the missing managed values.
	MissingValues []string
	// IsRequired indicates if these are required (vs checkpoint-related) values.
	IsRequired bool
}

// Error implements the error interface.
func (e *ManagedValueError) Error() string {
	if e.IsRequired {
		return fmt.Sprintf("graph: missing required managed values: %s", strings.Join(e.MissingValues, ", "))
	}
	return fmt.Sprintf("graph: checkpoint requires managed values (%s)", strings.Join(e.MissingValues, ", "))
}

// Is enables comparison with sentinel errors.
func (e *ManagedValueError) Is(target error) bool {
	_, ok := target.(*ManagedValueError)
	return ok
}

// RehydrateError represents an error that occurred while rehydrating a managed value.
// Use errors.As to extract the Name and Cause fields for programmatic handling.
type RehydrateError struct {
	// Name is the name of the managed value that failed to rehydrate.
	Name string
	// Cause is the underlying error.
	Cause error
}

// Error implements the error interface.
func (e *RehydrateError) Error() string {
	return fmt.Sprintf("graph: failed to rehydrate managed value %q: %v", e.Name, e.Cause)
}

// Unwrap returns the underlying error for errors.Unwrap.
func (e *RehydrateError) Unwrap() error {
	return e.Cause
}

// Is enables comparison with sentinel errors.
func (e *RehydrateError) Is(target error) bool {
	_, ok := target.(*RehydrateError)
	return ok
}
