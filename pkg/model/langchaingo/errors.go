// Package langchaingo provides sentinel errors for the LangChain Go model package.
package langchaingo

import "errors"

var (
	// ErrNoMessages is returned when generate is called without messages.
	ErrNoMessages = errors.New("model/langchaingo: generate requires at least one message")
)
