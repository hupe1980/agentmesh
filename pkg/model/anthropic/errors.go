// Package anthropic provides sentinel errors for the Anthropic model package.
package anthropic

import "errors"

var (
	// ErrNoMessages is returned when generate is called without messages.
	ErrNoMessages = errors.New("model/anthropic: generate requires at least one message")

	// ErrNoContent is returned when Anthropic returns no content.
	ErrNoContent = errors.New("model/anthropic: response contained no content")
)
