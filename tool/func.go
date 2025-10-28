package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

// Func is the function signature expected by FuncTool. Implementations receive:
//   - ctx: request-scoped context (cancellation, deadlines)
//   - toolCtx: AgentMesh ToolContext with session/run metadata
//   - args: already-validated arguments matching the declared Parameters schema
//
// The return value should be JSON-serializable.
type Func[T any] func(ctx context.Context, toolCtx core.ToolContext, args T) (any, error)

// FuncWithCredential is the function signature expected by FuncToolWithCredential. Implementations receive:
//   - ctx: request-scoped context (cancellation, deadlines)
//   - toolCtx: AgentMesh ToolContext with session/run metadata
//   - args: already-validated arguments matching the declared Parameters schema
//   - credential: Credential of type T associated with the tool call
//
// The return value should be JSON-serializable.
type FuncWithCredential[T any, C core.Credential] func(
	ctx context.Context,
	toolCtx core.ToolContext,
	args T,
	credential C,
) (any, error)

// FuncTool is a generic adapter that exposes a plain Go function as an AgentMesh tool.
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
// A FuncTool has no internal mutable state after construction and is safe for
// concurrent use by multiple goroutines.
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
type FuncTool[T any] struct {
	// Tool identifier (snake_case recommended)
	name string
	// Human-readable description shown to models
	description string
	// JSON schema describing accepted arguments
	parameters map[string]any
	// User supplied implementation
	fn Func[T]
}

// NewFuncTool constructs a FuncTool from explicit schema and function.
//
// Arguments:
//
//	name        - unique tool name (avoid collisions; snake_case suggested)
//	description - concise, imperative description ("Calculate the …")
//	parameters  - minimal JSON-Schema-like map describing the accepted arguments
//	fn          - implementation receiving a ToolContext plus already‑validated args
//
// Example:
//
//	sumTool := NewFuncTool(
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
//	  func(ctx context.Context, tc core.ToolContext, args map[string]any) (any, error) {
//	    a := args["a"].(float64)
//	    b := args["b"].(float64)
//	    return a + b, nil
//	  },
//	)
func NewFuncTool[T any](
	name, description string,
	parameters map[string]any,
	fn Func[T],
) *FuncTool[T] {
	return &FuncTool[T]{
		name:        name,
		description: description,
		parameters:  parameters,
		fn:          fn,
	}
}

// NewFuncToolFromType derives the parameter schema from a struct using jsonschema-go's
// inference (draft 2020-12). It respects json tags for names and omitempty for required.
//
// structType may be either:
//   - a struct value (e.g., SumArgs{}) or pointer to struct (e.g., &SumArgs{})
//   - a reflect.Type describing the struct type (e.g., reflect.TypeOf(SumArgs{}))
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
//	  func(ctx context.Context, tc core.ToolContext, args map[string]any) (any, error) {
//	    return args["a"].(float64) + args["b"].(float64), nil
//	  },
//	)
func NewFuncToolFromType[T any](
	name, description string,
	fn Func[T],
) (*FuncTool[T], error) {
	schema, err := jsonschema.MapFromStruct(*new(T))
	if err != nil {
		return nil, fmt.Errorf("NewFuncToolFromType: %w", err)
	}

	return NewFuncTool(name, description, schema, fn), nil
}

// Name returns the unique tool name used in function call declarations and routing.
func (t *FuncTool[T]) Name() string { return t.name }

// Description returns the short natural language description exposed to models.
func (t *FuncTool[T]) Description() string { return t.description }

// Parameters returns the (minimal) JSON schema describing expected arguments.
func (t *FuncTool[T]) Parameters() map[string]any { return t.parameters }

// IsLongRunning indicates whether the tool is a long-running operation.
func (t *FuncTool[T]) IsLongRunning() bool { return false }

// ProcessModelRequest adds this tool to the provided request.
func (t *FuncTool[T]) ProcessModelRequest(
	ctx context.Context,
	toolCtx core.ToolContext,
	req *core.ModelRequest,
) error {
	req.AddTool(t)
	return nil
}

// Call validates the provided args against the declared schema then invokes the
// underlying function. This implementation is intentionally minimal:
// - no logging (executor owns logging/timing)
// - no panic recovery (executor guarantees "never panic")
// - normalizes errors to *ToolError for consistent downstream handling
func (t *FuncTool[T]) Call(ctx context.Context, toolCtx core.ToolContext, args string) (any, error) {
	// Validate parameters against JSON Schema using helper (no fallback)
	if err := jsonschema.Validate(t.parameters, args); err != nil {
		return nil, NewError(t.name, err.Error(), "VALIDATION_ERROR")
	}

	var argsMap T
	if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
		return nil, NewError(t.name, fmt.Sprintf("invalid JSON arguments: %v", err), "VALIDATION_ERROR")
	}

	// Respect cancellation early
	select {
	case <-ctx.Done():
		return nil, NewError(t.name, fmt.Sprintf("canceled: %v", ctx.Err()), "EXECUTION_ERROR")
	default:
	}

	// Execute function
	res, err := t.fn(ctx, toolCtx, argsMap)
	if err != nil {
		// Pass through if already a *tool.Error, otherwise wrap
		if _, ok := err.(*Error); !ok {
			err = NewError(t.name, err.Error(), "EXECUTION_ERROR")
		}

		return nil, err
	}

	return res, nil
}

// Compile-time assertion: ensure FuncTool implements the core.Tool interface.
var _ core.Tool = (*FuncTool[any])(nil)
