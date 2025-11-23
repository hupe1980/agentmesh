package state

// deleteMarker is a sentinel value to indicate key deletion.
// Used by UpdateBuilder.Delete() in graph package to mark keys for removal.
type deleteMarker struct{}
