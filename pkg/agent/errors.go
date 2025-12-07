// Package agent provides sentinel errors for the agent package.
package agent

import "errors"

var (
	// ErrNoMessages is returned when there are no messages.
	ErrNoMessages = errors.New("agent: no messages")

	// ErrSessionIDRequired is returned when session_id is required but not provided.
	ErrSessionIDRequired = errors.New("agent/conversational: session_id is required")

	// ErrNoUserQuery is returned when no user query is found.
	ErrNoUserQuery = errors.New("agent/rag: no user query found")

	// ErrNoQueryMessages is returned when there are no query messages.
	ErrNoQueryMessages = errors.New("agent/rag: no query messages")

	// ErrNoMessagesInState is returned when there are no messages in state.
	ErrNoMessagesInState = errors.New("agent/rag: no messages in state")
)
