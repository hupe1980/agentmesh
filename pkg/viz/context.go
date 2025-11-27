package viz

// Package context utilities are minimal - viz reuses graph's context mechanisms.
// The viz package doesn't need its own context keys since it integrates with
// the graph event bus which already handles context propagation properly.
