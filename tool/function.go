package tool

import (
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
)

// FunctionTool is a generic adapter that exposes a plain Go function as an AgentMesh tool.
//
// Responsibilities:
//   - Holds a lightweight JSON-Schema-like parameter specification (parameters)
//   - Validates user / model supplied arguments against that schema before execution
//   - Invokes the wrapped function with a *core.ToolContext giving access to session state,
//     logging, function call IDs, artifact helpers, etc.
//   - Normalizes error handling so callers receive *ToolError with consistent codes:
//     VALIDATION_ERROR  -> schema / argument mismatch
//     EXECUTION_ERROR   -> underlying function returned an error (non-ToolError)
//     (custom codes preserved if the function returns *ToolError directly)
//
// Concurrency:
//
//	A FunctionTool has no internal mutable state after construction and is safe for
//	concurrent use by multiple goroutines.
//
// Parameter Schema Expectations:
//
//	The parameters map should follow a minimal JSON Schema shape used elsewhere in the
//	project. Only the subset actually validated by util.ValidateParameters needs to be
//	supplied (type, properties, required, enum, etc.).
//
// Returned result:
//
//	The returned value can be any Go type that is JSON‑serializable by the higher layer.
//	If more structure or streaming is required, create a custom Tool implementation instead.
type FunctionTool struct {
	// Tool identifier (snake_case recommended)
	name string
	// Human-readable description shown to models
	description string
	// JSON schema describing accepted arguments
	parameters map[string]any
	// User supplied implementation
	fn func(toolCtx *core.ToolContext, args map[string]any) (any, error)
}

// NewFunctionTool constructs a FunctionTool from explicit schema and function.
//
// Arguments:
//
//	name        - unique tool name (avoid collisions; snake_case suggested)
//	description - concise, imperative description ("Calculate the …")
//	parameters  - minimal JSON-Schema–like map describing the accepted arguments
//	fn          - implementation receiving a ToolContext plus already‑validated args
//
// Example:
//
//	sumTool := NewFunctionTool(
//	  "calculate_sum",
//	  "Calculate the sum of two numbers",
//	  map[string]any{
//	    "type": "object",
//	    "properties": map[string]any{
//	      "a": map[string]any{"type": "number"},
//	      "b": map[string]any{"type": "number"},
//	    },
//	    "required": []string{"a", "b"},
//	  },
//	  func(tc *core.ToolContext, args map[string]any) (any, error) {
//	    a := args["a"].(float64)
//	    b := args["b"].(float64)
//	    return a + b, nil
//	  },
//	)
func NewFunctionTool(
	name, description string,
	parameters map[string]any,
	fn func(toolCtx *core.ToolContext, args map[string]any) (any, error),
) *FunctionTool {
	return &FunctionTool{
		name:        name,
		description: description,
		parameters:  parameters,
		fn:          fn,
	}
}

// NewFunctionToolFromStruct derives the parameter schema from a struct using reflection.
// It is a convenience for simple argument containers and produces a schema equivalent
// to util.CreateSchema(structType).
//
// Example:
//
//	type SumArgs struct {
//	  A float64 `json:"a" description:"First addend"`
//	  B float64 `json:"b" description:"Second addend"`
//	}
//
//	sumTool := NewFunctionToolFromStruct(
//	  "calculate_sum",
//	  "Calculate the sum of two numbers",
//	  SumArgs{},
//	  func(tc *core.ToolContext, args map[string]any) (any, error) {
//	    return args["a"].(float64) + args["b"].(float64), nil
//	  },
//	)
func NewFunctionToolFromStruct(
	name, description string,
	structType any,
	fn func(toolCtx *core.ToolContext, args map[string]any) (any, error),
) *FunctionTool {
	schema := util.CreateSchema(structType)
	return NewFunctionTool(name, description, schema, fn)
}

// Name returns the unique tool name used in function call declarations and routing.
func (t *FunctionTool) Name() string { return t.name }

// Description returns the short natural language description exposed to models.
func (t *FunctionTool) Description() string { return t.description }

// Parameters returns the (minimal) JSON schema describing expected arguments.
func (t *FunctionTool) Parameters() map[string]any { return t.parameters }

// Call validates the provided args against the declared schema then invokes the
// underlying function. Validation or execution failures are wrapped (or passed
// through) as *ToolError for uniform downstream handling.
//
// Error Semantics:
//
//	*ToolError (returned directly)  -> forwarded unchanged
//	validation failure              -> *ToolError{Code: "VALIDATION_ERROR"}
//	other error                     -> *ToolError{Code: "EXECUTION_ERROR"}
//
// Logging Fields:
//
//	tool: tool name
//	fc_id: function call identifier (correlates model request & tool execution)
//	duration_ms: execution time in milliseconds
func (t *FunctionTool) Call(toolCtx *core.ToolContext, args map[string]any) (any, error) {
	logger := toolCtx.Logger()
	start := time.Now()

	logger.Debug("tool.call.start", "tool", t.name, "fc_id", toolCtx.FunctionCallID())

	if err := util.ValidateParameters(args, t.parameters); err != nil {
		logger.Warn("tool.call.validation_failed", "tool", t.name, "error", err.Error())

		return nil, &ToolError{
			Tool:    t.name,
			Message: fmt.Sprintf("parameter validation failed: %v", err),
			Code:    "VALIDATION_ERROR",
			Details: err,
		}
	}

	result, err := t.fn(toolCtx, args)
	if err != nil {
		if toolErr, ok := err.(*ToolError); ok { // Already a ToolError -> just log and forward
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
