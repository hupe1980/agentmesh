// Package memory provides sentinel errors for the memory package.
package memory

import "errors"

var (
	// ErrMissingMessageData is returned when message data is missing.
	ErrMissingMessageData = errors.New("memory: missing message data")

	// ErrMissingMessageType is returned when message type is missing.
	ErrMissingMessageType = errors.New("memory: missing message type")
)
