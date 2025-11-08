package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

// Func is the signature for tool implementation functions with typed arguments and results.
type Func[T any, R any] func(ctx context.Context, args T) (R, error)

// FuncTool wraps a Go function as a callable tool with JSON Schema validation.
// It provides type-safe tool implementations with automatic argument parsing.
type FuncTool[T any, R any] struct {
	// name is the tool identifier (snake_case recommended).
	name string
	// description is the human-readable description shown to models.
	description string
	// parameters is the JSON schema describing accepted arguments.
	parameters map[string]any
	// fn is the user-supplied implementation.
	fn Func[T, R]
}

// NewFuncTool creates a FuncTool with automatic JSON Schema generation from the argument type.
// The schema is inferred from the type parameter T using struct tags and field types.
//
// Example:
//
//	type SearchArgs struct {
//	    Query string `json:"query" jsonschema:"description=Search query"`
//	    Limit int    `json:"limit" jsonschema:"description=Max results"`
//	}
//
//	tool, err := tool.NewFuncTool("search", "Search for documents",
//	    func(ctx context.Context, args SearchArgs) (string, error) {
//	        return performSearch(args.Query, args.Limit)
//	    })
func NewFuncTool[T any, R any](
	name, description string,
	fn Func[T, R],
) (*FuncTool[T, R], error) {
	if name == "" {
		return nil, fmt.Errorf("tool name must not be empty")
	}
	if fn == nil {
		return nil, fmt.Errorf("tool function must not be nil")
	}

	schema, err := jsonschema.MapFromStruct(*new(T))
	if err != nil {
		return nil, fmt.Errorf("NewFuncToolFromType: %w", err)
	}

	return NewFuncToolFromMap(name, description, schema, fn), nil
}

// MustNewFuncTool is like NewFuncTool but panics on error.
// Use this in tests or when you're certain inputs are valid.
func MustNewFuncTool[T any, R any](
	name, description string,
	fn Func[T, R],
) *FuncTool[T, R] {
	tool, err := NewFuncTool(name, description, fn)
	if err != nil {
		panic(err)
	}
	return tool
}

// NewFuncToolFromMap creates a FuncTool with an explicit JSON Schema provided as a map.
// Use this when you need fine-grained control over the schema or when the automatic
// schema generation from NewFuncTool doesn't meet your needs.
//
// Example:
//
//	schema := map[string]any{
//	    "type": "object",
//	    "properties": map[string]any{
//	        "query": map[string]any{
//	            "type": "string",
//	            "description": "Search query",
//	        },
//	    },
//	    "required": []string{"query"},
//	}
//	tool := tool.NewFuncToolFromMap("search", "Search documents", schema, searchFunc)
func NewFuncToolFromMap[T any, R any](name, description string, parameters map[string]any, fn Func[T, R]) *FuncTool[T, R] {
	return &FuncTool[T, R]{name, description, parameters, fn}
}

// Name returns the unique tool name used in function call declarations and routing.
func (t *FuncTool[T, R]) Name() string { return t.name }

// Description returns the short natural language description exposed to models.
func (t *FuncTool[T, R]) Description() string { return t.description }

// Definition returns the tool definition including the JSON Schema for arguments.
func (t *FuncTool[T, R]) Definition() *ToolDefinition {
	return &ToolDefinition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        t.name,
			Description: t.description,
			Parameters:  t.parameters,
		},
	}
}

// Call executes the tool function with JSON-serialized arguments.
// It validates arguments against the JSON Schema, deserializes them to type T,
// and invokes the wrapped function. Returns an error if validation fails,
// deserialization fails, or the function returns an error.
func (t *FuncTool[T, R]) Call(ctx context.Context, args string) (any, error) {
	// Validate parameters against JSON Schema using helper (no fallback)
	if err := jsonschema.Validate(t.parameters, args); err != nil {
		return nil, fmt.Errorf("tool %q: invalid arguments: %w", t.name, err)
	}

	var parsedArgs T
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return nil, fmt.Errorf("tool %q: invalid JSON arguments: %w", t.name, err)
	}

	// Respect cancellation early
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("tool %q: canceled: %w", t.name, ctx.Err())
	default:
	}

	return t.fn(ctx, parsedArgs)
}
