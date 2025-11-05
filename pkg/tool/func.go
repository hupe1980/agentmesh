package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

type Func[T any, R any] func(ctx context.Context, args T) (R, error)

type FuncTool[T any, R any] struct {
	// Tool identifier (snake_case recommended)
	name string
	// Human-readable description shown to models
	description string
	// JSON schema describing accepted arguments
	parameters map[string]any
	// User supplied implementation
	fn Func[T, R]
}

func NewFuncTool[T any, R any](
	name, description string,
	fn Func[T, R],
) (*FuncTool[T, R], error) {
	schema, err := jsonschema.MapFromStruct(*new(T))
	if err != nil {
		return nil, fmt.Errorf("NewFuncToolFromType: %w", err)
	}

	return NewFuncToolFromMap(name, description, schema, fn), nil
}

func NewFuncToolFromMap[T any, R any](name, description string, parameters map[string]any, fn Func[T, R]) *FuncTool[T, R] {
	return &FuncTool[T, R]{name, description, parameters, fn}
}

// Name returns the unique tool name used in function call declarations and routing.
func (t *FuncTool[T, R]) Name() string { return t.name }

// Description returns the short natural language description exposed to models.
func (t *FuncTool[T, R]) Description() string { return t.description }

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
