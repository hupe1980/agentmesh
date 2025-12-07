// Package floatconv provides utilities for converting between float32 and float64 slices.
package floatconv

// ToFloat32 converts a float64 slice to a float32 slice.
func ToFloat32(v []float64) []float32 {
	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = float32(val)
	}
	return result
}

// ToFloat64 converts a float32 slice to a float64 slice.
func ToFloat64(v []float32) []float64 {
	result := make([]float64, len(v))
	for i, val := range v {
		result[i] = float64(val)
	}
	return result
}

// ToFloat64FromAny converts an []any slice (typically from JSON unmarshaling) to []float64.
// Non-numeric values are converted to 0.
func ToFloat64FromAny(v []any) []float64 {
	result := make([]float64, len(v))
	for i, val := range v {
		switch f := val.(type) {
		case float64:
			result[i] = f
		case float32:
			result[i] = float64(f)
		case int:
			result[i] = float64(f)
		case int64:
			result[i] = float64(f)
		}
	}
	return result
}
