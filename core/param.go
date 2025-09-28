package core

// String creates an optional string value (Opt[string]) set to v.
// Zero value of Opt[string] represents None.
func String(v string) Opt[string] {
	return Some(v)
}

// Int creates an optional int value (Opt[int]) set to v.
// Zero value of Opt[int] represents None.
func Int(v int) Opt[int] {
	return Some(v)
}

// Int64 creates an optional int64 value (Opt[int64]) set to v.
// Zero value of Opt[int64] represents None.
func Int64(v int64) Opt[int64] {
	return Some(v)
}

// Float64 creates an optional float64 value (Opt[float64]) set to v.
// Zero value of Opt[float64] represents None.
func Float64(v float64) Opt[float64] {
	return Some(v)
}

// Bool creates an optional bool value (Opt[bool]) set to v.
// Zero value of Opt[bool] represents None.
func Bool(v bool) Opt[bool] {
	return Some(v)
}

// Map creates an Opt[map[K]V] from a map, marking it as set.
func Map[K comparable, V any](m map[K]V) Opt[map[K]V] {
	if m == nil {
		return None[map[K]V]()
	}

	return Some(m)
}
