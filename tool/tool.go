// Package tool implements the function / tool calling subsystem that lets agents
// invoke structured capabilities (APIs, computations, side‑effects) with schema
// validated arguments, consistent error handling and rich metadata for LLM guidance.
package tool

import (
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
)

// Tool defines the interface for extending agent capabilities with external functions.
//
// Tools can be registered with agents to enable function calling, allowing
// agents to perform actions beyond text generation such as API calls, calculations,
// database queries, or any other programmatic operations.
//
// All tools have access to ToolContext for session state, agent flow control,
// authentication, memory, and artifact management. This enables tools to build
// sophisticated workflows and integrate deeply with the AgentMesh framework infrastructure.
//
// Tool implementations should:
//   - Provide clear, descriptive names and descriptions
//   - Define proper JSON schema for parameters
//   - Handle errors gracefully
//   - Be thread-safe if used concurrently
//   - Follow consistent naming conventions
type Tool interface {
	// Name returns the unique identifier for this tool.
	// Names should be descriptive and follow function naming conventions (snake_case recommended).
	Name() string

	// Description returns a human-readable description of what this tool does.
	// This description is provided to the LLM to help it understand when and how to use the tool.
	Description() string

	// Parameters returns a JSON schema describing the expected input format.
	// This schema is used for parameter validation and LLM function calling.
	Parameters() map[string]interface{}

	// Call executes the tool with structured arguments and ToolContext.
	// This method provides tools with access to session state, agent actions,
	// authentication, memory, and artifact management capabilities.
	// Arguments are parsed from JSON and validated against the tool's schema.
	Call(toolCtx *core.ToolContext, args map[string]interface{}) (interface{}, error)
}

// ValidationError represents parameter validation errors with detailed information.
type ValidationError = util.ValidationError

// ToolError represents errors that occur during tool execution.
type ToolError struct {
	Tool    string      `json:"tool"`              // Name of the tool that failed
	Message string      `json:"message"`           // Error message
	Code    string      `json:"code"`              // Error code for categorization
	Details interface{} `json:"details,omitempty"` // Additional error details
}

func (e *ToolError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("tool error [%s] in %s: %s", e.Code, e.Tool, e.Message)
	}
	return fmt.Sprintf("tool error in %s: %s", e.Tool, e.Message)
}

// NewToolError creates a new ToolError with the specified details.
func NewToolError(tool, message, code string) *ToolError {
	return &ToolError{
		Tool:    tool,
		Message: message,
		Code:    code,
	}
}
