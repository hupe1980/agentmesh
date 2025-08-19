package util

import "github.com/google/uuid"

// NewID generates a new UUID.
func NewID() string { return uuid.NewString() }
