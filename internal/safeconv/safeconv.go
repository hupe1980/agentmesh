// Package safeconv provides safe integer conversions that handle overflow.
package safeconv

import "math"

// IntToInt32 safely converts an int to int32, clamping to int32 bounds.
func IntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}

	if v < math.MinInt32 {
		return math.MinInt32
	}

	return int32(v)
}

// IntToUint32 safely converts an int to uint32, clamping to uint32 bounds.
func IntToUint32(v int) uint32 {
	if v < 0 {
		return 0
	}

	if v > math.MaxUint32 {
		return math.MaxUint32
	}

	return uint32(v)
}

// IntToUint64 safely converts an int to uint64, clamping negative values to 0.
func IntToUint64(v int) uint64 {
	if v < 0 {
		return 0
	}

	return uint64(v)
}

// Uint64ToInt64 safely converts a uint64 to int64, clamping to int64 max.
func Uint64ToInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}

	return int64(v)
}
