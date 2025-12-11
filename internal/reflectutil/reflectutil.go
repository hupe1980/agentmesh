package reflectutil

import "reflect"

// IsNilOrZero checks if a value is nil, zero, or empty for its type.
// For slices and maps, empty (len=0) is treated as "no input provided".
func IsNilOrZero[T any](v T) bool {
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return true
	}
	switch val.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Func, reflect.Interface:
		return val.IsNil()
	case reflect.Slice, reflect.Map:
		return val.IsNil() || val.Len() == 0
	}
	return val.IsZero()
}

// IsNil checks if a value is nil. Unlike IsNilOrZero, zero values for
// non-nilable types (e.g., 0, false) are treated as provided input.
func IsNil[T any](v T) bool {
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return true
	}
	switch val.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Func, reflect.Interface, reflect.Slice, reflect.Map:
		return val.IsNil()
	}
	return false
}
