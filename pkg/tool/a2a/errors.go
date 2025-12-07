// Package a2a provides sentinel errors for the a2a tool package.
package a2a

import "errors"

var (
	// ErrUnexpectedResponse is returned when A2A response type is unexpected.
	ErrUnexpectedResponse = errors.New("tool/a2a: unexpected response type from a2a agent")
)
