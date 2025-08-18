package artifact

import "fmt"

var (
	// ErrNotFound is returned when an artifact for the given session / id pair
	// does not exist in the underlying store.
	ErrNotFound = fmt.Errorf("artifact not found")
)
