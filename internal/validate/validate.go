// Package validate provides reusable validation helpers to standardize error
// messages and reduce code duplication across the agentmesh codebase.
package validate

import (
	"fmt"
	"reflect"
)

// NotNil checks if a value (pointer, interface, slice, map, channel, or function) is nil
// and returns a standardized error if so.
func NotNil[T any](v T, name string) error {
	if isNil(v) {
		return fmt.Errorf("%s must not be nil", name)
	}
	return nil
}

// isNil checks if a value is nil, handling pointers, interfaces, and other nullable types.
func isNil(v any) bool {
	if v == nil {
		return true
	}

	// Use reflection to check if the underlying value is nil
	// This is necessary for interface types where the interface itself is non-nil
	// but the underlying value is nil
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}

// NotEmpty checks if a string is empty and returns a standardized error if so.
func NotEmpty(s, name string) error {
	if s == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	return nil
}

// NotEmptySlice checks if a slice is empty and returns a standardized error if so.
func NotEmptySlice[T any](slice []T, name string) error {
	if len(slice) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	return nil
}

// All validates multiple conditions and returns the first error encountered.
// This is useful for chaining multiple validations together.
func All(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
