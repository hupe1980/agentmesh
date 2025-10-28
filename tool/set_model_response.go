package tool

import (
	"context"
	"fmt"
	"maps"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

// SetModelResponseTool is an internal tool used when output_schema is configured
// alongside other tools. It lets the model provide its final structured response.
type SetModelResponseTool struct {
	name         string
	description  string
	outputSchema map[string]any
}

const setModelResponseInstruction = "IMPORTANT: You have access to other tools, but you must provide your final " +
	"response using the set_model_response tool with the required structured format. After using any other tools needed " +
	"to complete the task, always call set_model_response with your final answer in the specified schema format."

// NewSetModelResponseTool creates a new tool with the given struct schema.
// Example: NewSetModelResponseTool(MyOutputSchema{})
func NewSetModelResponseTool(outputSchema any) (*SetModelResponseTool, error) {
	var schema map[string]any

	switch s := outputSchema.(type) {
	case nil:
		return nil, fmt.Errorf("NewSetModelResponseTool: nil output schema")
	case map[string]any:
		schema = maps.Clone(s)
	case core.OutputSchema:
		schema = maps.Clone(s.Schema)
	case *core.OutputSchema:
		if s == nil {
			return nil, fmt.Errorf("NewSetModelResponseTool: nil output schema pointer")
		}
		schema = maps.Clone(s.Schema)
	default:
		converted, err := jsonschema.MapFromStruct(outputSchema)
		if err != nil {
			return nil, fmt.Errorf("NewSetModelResponseTool: %w", err)
		}
		schema = converted
	}

	return &SetModelResponseTool{
		name: "set_model_response",
		description: "Set your final response using the required output schema. " +
			"Use this tool to provide your final structured answer instead of outputting text directly.",
		outputSchema: schema,
	}, nil
}

// Name returns the tool's unique identifier.
func (t *SetModelResponseTool) Name() string {
	return t.name
}

// Description returns the tool's description.
func (t *SetModelResponseTool) Description() string {
	return t.description
}

// Parameters returns the tool's parameter schema.
func (t *SetModelResponseTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": t.outputSchema["properties"],
		"required":   t.outputSchema["required"],
	}
}

// IsLongRunning indicates whether the tool is a long-running operation.
func (t *SetModelResponseTool) IsLongRunning() bool {
	return false
}

// ProcessModelRequest implements core.Tool. It registers itself with the request.
func (t *SetModelResponseTool) ProcessModelRequest(
	ctx context.Context,
	toolCtx core.ToolContext,
	req *core.ModelRequest,
) error {
	req.AddTool(t)
	req.AppendInstructions(setModelResponseInstruction)

	return nil
}

// Call implements core.Tool. It simply returns the provided arguments for
// further processing by the flow.
func (t *SetModelResponseTool) Call(ctx context.Context, toolCtx core.ToolContext, args string) (any, error) {
	// Validate args against schema
	if err := jsonschema.Validate(t.outputSchema, args); err != nil {
		return nil, fmt.Errorf("invalid model response: %w", err)
	}

	return args, nil
}

// Compile-time assertion
var _ core.Tool = (*SetModelResponseTool)(nil)
