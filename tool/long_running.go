package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

const longRunningInstruction = `

NOTE: This is a long-running operation.
Do not call this tool again if it has already returned some intermediate or pending status.`

// LongRunningTool wraps a FuncTool and annotates its metadata as long-running.
type LongRunningTool[T any] struct {
	*FuncTool[T]
}

// NewLongRunningTool creates a FuncTool variant that warns callers about long execution times.
func NewLongRunningTool[T any](
	name, description string,
	parameters map[string]any,
	fn Func[T],
) *LongRunningTool[T] {
	return &LongRunningTool[T]{
		FuncTool: NewFuncTool(name, description, parameters, fn),
	}
}

// NewLongRunningToolFromType derives the parameter schema from a struct using the same helper as FuncTool.
func NewLongRunningToolFromType[T any](
	name, description string,
	fn Func[T],
) (*LongRunningTool[T], error) {
	schema, err := jsonschema.MapFromStruct(*new(T))
	if err != nil {
		return nil, fmt.Errorf("NewLongRunningToolFromType: %w", err)
	}

	return NewLongRunningTool(name, description, schema, fn), nil
}

// Description returns the base description plus a long-running instruction hint.
func (t *LongRunningTool[T]) Description() string {
	desc := t.FuncTool.Description()
	if desc == "" {
		return strings.TrimPrefix(longRunningInstruction, "\n\n")
	}

	return desc + longRunningInstruction
}

// ProcessModelRequest ensures the long-running wrapper is registered with the request.
func (t *LongRunningTool[T]) ProcessModelRequest(
	ctx context.Context,
	toolCtx core.ToolContext,
	req *core.ModelRequest,
) error {
	req.AddTool(t)
	return nil
}

// IsLongRunning indicates that this tool may take a long time to complete.
func (t *LongRunningTool[T]) IsLongRunning() bool {
	return true
}
