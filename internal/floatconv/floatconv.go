// Package floatconv provides utilities for converting between float32 and float64 slices.
package floatconv

// ToFloat32 converts a float64 slice to a float32 slice.
// This is commonly used at API boundaries where SDKs return float64
// but float32 is preferred for storage and computation.
func ToFloat32(v []float64) []float32 {
	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = float32(val)
	}
	return result
}

// ToFloat32FromAny converts an []any slice (typically from JSON unmarshaling) to []float32.
// Non-numeric values are converted to 0.
func ToFloat32FromAny(v []any) []float32 {
	result := make([]float32, len(v))
	for i, val := range v {
		switch f := val.(type) {
		case float64:
			result[i] = float32(f)
		case float32:
			result[i] = f
		case int:
			result[i] = float32(f)
		case int64:
			result[i] = float32(f)
		}
	}
	return result
}
