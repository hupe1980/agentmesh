package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
)

// FunctionTool provides a convenient implementation of the Tool interface
// for wrapping Go functions as agent tools.
//
// This implementation handles JSON parameter parsing, validation, and provides
// a clean interface for creating tools from existing Go functions with automatic
// schema generation and comprehensive error handling.
type FunctionTool struct {
	name        string                                                                      // Tool identifier
	description string                                                                      // Human-readable description
	parameters  map[string]interface{}                                                      // JSON schema for parameters
	fn          func(ctx context.Context, args map[string]interface{}) (interface{}, error) // Implementation function
	metadata    *functionToolMetadata                                                       // Additional metadata
}

// functionToolMetadata implements ToolMetadata for FunctionTool.
type functionToolMetadata struct {
	version  string
	category string
	tags     []string
	examples []ToolExample
}

func (m *functionToolMetadata) Version() string         { return m.version }
func (m *functionToolMetadata) Category() string        { return m.category }
func (m *functionToolMetadata) Tags() []string          { return m.tags }
func (m *functionToolMetadata) Examples() []ToolExample { return m.examples }

// FunctionToolOption allows customization of FunctionTool behavior.
type FunctionToolOption func(*FunctionTool)

// WithVersion sets the tool version.
func WithVersion(version string) FunctionToolOption {
	return func(t *FunctionTool) {
		if t.metadata == nil {
			t.metadata = &functionToolMetadata{}
		}
		t.metadata.version = version
	}
}

// WithCategory sets the tool category.
func WithCategory(category string) FunctionToolOption {
	return func(t *FunctionTool) {
		if t.metadata == nil {
			t.metadata = &functionToolMetadata{}
		}
		t.metadata.category = category
	}
}

// WithTags sets descriptive tags for the tool.
func WithTags(tags ...string) FunctionToolOption {
	return func(t *FunctionTool) {
		if t.metadata == nil {
			t.metadata = &functionToolMetadata{}
		}
		t.metadata.tags = tags
	}
}

// WithExamples adds usage examples to the tool.
func WithExamples(examples ...ToolExample) FunctionToolOption {
	return func(t *FunctionTool) {
		if t.metadata == nil {
			t.metadata = &functionToolMetadata{}
		}
		t.metadata.examples = examples
	}
}

// WithValidation enables parameter validation using the provided schema.
func WithValidation(validate bool) FunctionToolOption {
	// This could be expanded to add custom validation logic
	return func(t *FunctionTool) {
		// Validation is enabled by default
	}
}

// NewFunctionTool creates a new FunctionTool with the specified configuration.
//
// Parameters:
//   - name: Unique identifier for the tool (should follow snake_case convention)
//   - description: Human-readable description for the LLM
//   - parameters: JSON schema defining expected input structure
//   - fn: Implementation function that processes the tool call
//   - opts: Optional configuration functions
//
// The implementation function receives parsed JSON arguments as a map and
// should return the result or an error. The result will be converted to a
// string representation for the agent.
//
// Example:
//
//	tool := NewFunctionTool(
//	  "calculate_sum",
//	  "Calculates the sum of two numbers",
//	  map[string]interface{}{
//	    "type": "object",
//	    "properties": map[string]interface{}{
//	      "a": map[string]interface{}{"type": "number"},
//	      "b": map[string]interface{}{"type": "number"},
//	    },
//	    "required": []string{"a", "b"},
//	  },
//	  func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
//	    a, _ := args["a"].(float64)
//	    b, _ := args["b"].(float64)
//	    return a + b, nil
//	  },
//	  WithCategory("math"),
//	  WithTags("arithmetic", "calculator"),
//	)
func NewFunctionTool(
	name, description string,
	parameters map[string]interface{},
	fn func(ctx context.Context, args map[string]interface{}) (interface{}, error),
	opts ...FunctionToolOption,
) *FunctionTool {
	tool := &FunctionTool{
		name:        name,
		description: description,
		parameters:  parameters,
		fn:          fn,
	}

	for _, opt := range opts {
		opt(tool)
	}

	return tool
}

// NewFunctionToolFromStruct creates a FunctionTool with automatic schema generation from a struct.
//
// This is a convenience function that uses reflection to generate the JSON schema
// from a Go struct type. The struct should have appropriate json tags and descriptions.
//
// Example:
//
//	type CalculateArgs struct {
//	  A float64 `json:"a" description:"First number"`
//	  B float64 `json:"b" description:"Second number"`
//	}
//
//	tool := NewFunctionToolFromStruct(
//	  "calculate_sum",
//	  "Calculates the sum of two numbers",
//	  CalculateArgs{},
//	  func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
//	    // Implementation
//	  },
//	)
func NewFunctionToolFromStruct(
	name, description string,
	structType interface{},
	fn func(ctx context.Context, args map[string]interface{}) (interface{}, error),
	opts ...FunctionToolOption,
) *FunctionTool {
	schema := util.CreateSchema(structType)
	return NewFunctionTool(name, description, schema, fn, opts...)
}

// Name returns the tool's unique identifier.
func (t *FunctionTool) Name() string {
	return t.name
}

// Description returns the tool's human-readable description.
func (t *FunctionTool) Description() string {
	return t.description
}

// Parameters returns the JSON schema for the tool's expected input.
func (t *FunctionTool) Parameters() map[string]interface{} {
	return t.parameters
}

// Call executes the tool with parsed arguments and ToolContext.
// This method bypasses JSON parsing and provides direct access to ToolContext capabilities.
func (t *FunctionTool) Call(toolCtx *core.ToolContext, args map[string]interface{}) (interface{}, error) {
	logger := toolCtx.Logger()
	start := time.Now()
	logger.Debug("tool.call.start", "tool", t.name, "fc_id", toolCtx.FunctionCallID())
	// Validate parameters against schema
	if err := util.ValidateParameters(args, t.parameters); err != nil {
		logger.Warn("tool.call.validation_failed", "tool", t.name, "error", err.Error())
		return nil, &ToolError{
			Tool:    t.name,
			Message: fmt.Sprintf("parameter validation failed: %v", err),
			Code:    "VALIDATION_ERROR",
			Details: err,
		}
	}

	// Execute the tool function with context from ToolContext
	result, err := t.fn(toolCtx.Context(), args)
	if err != nil {
		// Check if it's already a ToolError
		if toolErr, ok := err.(*ToolError); ok {
			logger.Error("tool.call.error", "tool", t.name, "error", toolErr.Message)
			return nil, toolErr
		}
		logger.Error("tool.call.error", "tool", t.name, "error", err.Error())
		return nil, &ToolError{
			Tool:    t.name,
			Message: err.Error(),
			Code:    "EXECUTION_ERROR",
		}
	}
	logger.Info("tool.call.success", "tool", t.name, "duration_ms", time.Since(start).Milliseconds())
	return result, nil
}

// GetMetadata returns the tool's metadata if available.
func (t *FunctionTool) GetMetadata() ToolMetadata {
	return t.metadata
}
