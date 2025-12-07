// Package ollama provides sentinel errors for the Ollama model package.
package ollama

import "errors"

var (
	// ErrYieldFalse is returned when a yield function returns false.
	ErrYieldFalse = errors.New("model/ollama: yield returned false")

	// ErrInvalidToolMessage is returned when a tool message is invalid.
	ErrInvalidToolMessage = errors.New("model/ollama: invalid tool message")
)
