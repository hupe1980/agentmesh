package tool

import (
	"fmt"
)

// Error represents errors that occur during tool execution.
type Error struct {
	Tool    string `json:"tool"`              // Name of the tool that failed
	Message string `json:"message"`           // Error message
	Code    string `json:"code"`              // Error code for categorization
	Details any    `json:"details,omitempty"` // Additional error details
}

// NewError creates a new Error with the specified details.
func NewError(tool, message, code string) *Error {
	return &Error{
		Tool:    tool,
		Message: message,
		Code:    code,
	}
}

// Error implements the error interface for Tool errors.
func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("tool error [%s] in %s: %s", e.Code, e.Tool, e.Message)
	}

	return fmt.Sprintf("tool error in %s: %s", e.Tool, e.Message)
}
