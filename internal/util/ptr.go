package util

// AddrOrNil returns nil if x is the zero value for T,
// or &x otherwise.
func AddrOrNil[T comparable](x T) *T {
	var z T
	if x == z {
		return nil
	}

	return &x
}

// PTR returns a pointer to the given value x.
func PTR[T comparable](x T) *T {
	return &x
}

// ClonePTR returns a new pointer with the same value as p, or nil if p is nil.
func ClonePTR[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
