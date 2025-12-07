// Package gemini provides sentinel errors for the Gemini model package.
package gemini

import "errors"

var (
	// ErrNoMessages is returned when generate is called without messages.
	ErrNoMessages = errors.New("model/gemini: generate requires at least one message")

	// ErrNoContent is returned when Gemini returns no content.
	ErrNoContent = errors.New("model/gemini: response contained no content")

	// ErrNoCandidates is returned when Gemini returns no candidates.
	ErrNoCandidates = errors.New("model/gemini: response contained no candidates")
)
