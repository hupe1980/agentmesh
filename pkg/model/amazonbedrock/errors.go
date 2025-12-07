// Package amazonbedrock provides sentinel errors for the Amazon Bedrock model package.
package amazonbedrock

import "errors"

var (
	// ErrNoMessages is returned when generate is called without messages.
	ErrNoMessages = errors.New("model/amazonbedrock: generate requires at least one message")
)
