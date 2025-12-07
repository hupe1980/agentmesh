// Package a2a provides sentinel errors for the a2a package.
package a2a

import "errors"

var (
	// ErrEmptyAgentCardURL is returned when agentCardURL is empty.
	ErrEmptyAgentCardURL = errors.New("a2a: agentCardURL cannot be empty")

	// ErrEmptySkillID is returned when skillID is empty.
	ErrEmptySkillID = errors.New("a2a: skillID cannot be empty")

	// ErrNoTaskContent is returned when task has no message content.
	ErrNoTaskContent = errors.New("a2a: task has no message content (no artifacts, status message, or history)")

	// ErrNilMessage is returned when message is nil.
	ErrNilMessage = errors.New("a2a: message cannot be nil")

	// ErrNilA2AMessage is returned when a2a message is nil.
	ErrNilA2AMessage = errors.New("a2a: a2a message cannot be nil")
)
