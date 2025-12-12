package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/guardrail"
)

// Func is the signature for tool implementation functions with typed arguments and results.
type Func[T any, R any] func(ctx context.Context, args T) (R, error)

// FuncToolOptions configures a FuncTool.
type FuncToolOptions struct {
	// InputGuardrails are applied to arguments before tool execution.
	InputGuardrails []guardrail.Guardrail[string]
	// OutputGuardrails are applied to the result after tool execution.
	OutputGuardrails []guardrail.Guardrail[string]
}

// FuncToolOption configures FuncTool options.
type FuncToolOption func(*FuncToolOptions)

// WithInputGuardrails adds input guardrails that validate tool arguments.
func WithInputGuardrails(guardrails ...guardrail.Guardrail[string]) FuncToolOption {
	return func(o *FuncToolOptions) {
		o.InputGuardrails = append(o.InputGuardrails, guardrails...)
	}
}

// WithOutputGuardrails adds output guardrails that validate tool results.
func WithOutputGuardrails(guardrails ...guardrail.Guardrail[string]) FuncToolOption {
	return func(o *FuncToolOptions) {
		o.OutputGuardrails = append(o.OutputGuardrails, guardrails...)
	}
}

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
	// inputGuardrails validate arguments before execution.
	inputGuardrails []guardrail.Guardrail[string]
	// outputGuardrails validate results after execution.
	outputGuardrails []guardrail.Guardrail[string]
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
//	    },
//	    tool.WithInputGuardrails(contentFilter),
//	)
func NewFuncTool[T any, R any](
	name, description string,
	fn Func[T, R],
	opts ...FuncToolOption,
) (*FuncTool[T, R], error) {
	if err := validate.All(
		validate.NotEmpty(name, "tool name"),
		validate.NotNil(fn, "tool function"),
	); err != nil {
		return nil, err
	}

	schema, err := jsonschema.MapFromStruct(*new(T))
	if err != nil {
		return nil, fmt.Errorf("tool/func: create from type: %w", err)
	}

	options := &FuncToolOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return &FuncTool[T, R]{
		name:             name,
		description:      description,
		fn:               fn,
		parameters:       schema,
		inputGuardrails:  options.InputGuardrails,
		outputGuardrails: options.OutputGuardrails,
	}, nil
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
func NewFuncToolFromMap[T any, R any](name, description string, parameters map[string]any, fn Func[T, R], opts ...FuncToolOption) *FuncTool[T, R] {
	options := &FuncToolOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return &FuncTool[T, R]{
		name:             name,
		description:      description,
		parameters:       parameters,
		fn:               fn,
		inputGuardrails:  options.InputGuardrails,
		outputGuardrails: options.OutputGuardrails,
	}
}

// Name returns the unique tool name used in function call declarations and routing.
func (t *FuncTool[T, R]) Name() string { return t.name }

// Description returns the short natural language description exposed to models.
func (t *FuncTool[T, R]) Description() string { return t.description }

// Definition returns the tool definition with schema.
func (t *FuncTool[T, R]) Definition() *Definition {
	return &Definition{
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
// runs input guardrails, invokes the wrapped function, and runs output guardrails.
// Returns an error if validation fails, guardrails reject, or the function returns an error.
func (t *FuncTool[T, R]) Call(ctx context.Context, args string) (any, error) {
	// Validate parameters against JSON Schema using helper (no fallback)
	if err := jsonschema.Validate(t.parameters, args); err != nil {
		return nil, fmt.Errorf("tool/func %q: invalid arguments: %w", t.name, err)
	}

	// Run input guardrails on arguments
	if len(t.inputGuardrails) > 0 {
		result, err := guardrail.Chain(ctx, args, t.inputGuardrails...)
		if err != nil {
			return nil, fmt.Errorf("tool/func %q: input guardrail error: %w", t.name, err)
		}

		if result.IsTripwire() {
			return nil, guardrail.NewTripwireError(t.name+":input", result)
		}

		if !result.IsAllowed() {
			return nil, fmt.Errorf("tool/func %q: input rejected: %s", t.name, result.Message)
		}
	}

	var parsedArgs T
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return nil, fmt.Errorf("tool/func %q: invalid JSON arguments: %w", t.name, err)
	}

	// Respect cancellation early
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("tool/func %q: canceled: %w", t.name, ctx.Err())
	default:
	}

	output, err := t.fn(ctx, parsedArgs)
	if err != nil {
		return nil, err
	}

	// Run output guardrails on result
	if len(t.outputGuardrails) > 0 {
		outputStr := fmt.Sprintf("%v", output)
		result, err := guardrail.Chain(ctx, outputStr, t.outputGuardrails...)
		if err != nil {
			return nil, fmt.Errorf("tool/func %q: output guardrail error: %w", t.name, err)
		}

		if result.IsTripwire() {
			return nil, guardrail.NewTripwireError(t.name+":output", result)
		}

		if !result.IsAllowed() {
			return nil, fmt.Errorf("tool/func %q: output rejected: %s", t.name, result.Message)
		}
	}

	return output, nil
}
