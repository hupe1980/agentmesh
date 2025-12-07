// Package openai provides sentinel errors for the OpenAI model package.
package openai

import "errors"

var (
	// ErrNoMessages is returned when generate is called without messages.
	ErrNoMessages = errors.New("model/openai: generate requires at least one message")

	// ErrNoChoices is returned when OpenAI returns no choices.
	ErrNoChoices = errors.New("model/openai: chat completion returned no choices")

	// ErrEmptyMessage is returned when OpenAI returns an empty message.
	ErrEmptyMessage = errors.New("model/openai: chat completion returned empty message")
)
