package tool

import (
	"context"
	"fmt"
	"maps"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
	"github.com/hupe1980/agentmesh/pkg/schema"
)

const setModelResponseInstruction = "IMPORTANT: You have access to other tools, but you must provide your final " +
	"response using the set_model_response tool with the required structured format. After using any other tools needed " +
	"to complete the task, always call set_model_response with your final answer in the specified schema format."

// SetModelResponseTool is an internal tool used when structured output is configured
// alongside other tools. It lets the model provide its final structured response
// using tool calling, enabling structured output on models without native support.
//
// This is the "tool trick" - converting a schema into a tool that the model must
// call to provide its final response.
type SetModelResponseTool struct {
	name         string
	description  string
	outputSchema map[string]any
}

// NewSetModelResponseTool creates a new tool with the given schema.
// The schema parameter can be:
//   - A struct type (generates schema via reflection)
//   - A map[string]any (uses directly as schema)
//   - A *schema.OutputSchema (extracts Schema field)
//
// Example with struct:
//
//	type AnalysisResult struct {
//	    Category   string  `json:"category" jsonschema:"required"`
//	    Confidence float64 `json:"confidence" jsonschema:"required"`
//	}
//	tool, err := tool.NewSetModelResponseTool(AnalysisResult{})
//
// Example with OutputSchema:
//
//	outputSchema, _ := schema.NewOutputSchema("result", MyStruct{})
//	tool, err := tool.NewSetModelResponseTool(&outputSchema)
//
// The tool automatically:
//   - Validates the response against the schema
//   - Adds instructions to the model request
//   - Returns the validated response for further processing
func NewSetModelResponseTool(outputSchema any) (*SetModelResponseTool, error) {
	var schemaMap map[string]any

	switch s := outputSchema.(type) {
	case nil:
		return nil, fmt.Errorf("tool/set_model_response: nil output schema")
	case map[string]any:
		schemaMap = maps.Clone(s)
	case schema.OutputSchema:
		schemaMap = maps.Clone(s.Schema)
	case *schema.OutputSchema:
		if s == nil {
			return nil, fmt.Errorf("tool/set_model_response: nil output schema pointer")
		}
		schemaMap = maps.Clone(s.Schema)
	default:
		// Try to convert as struct
		converted, err := jsonschema.MapFromStruct(outputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool/set_model_response: %w", err)
		}
		schemaMap = converted
	}

	return &SetModelResponseTool{
		name: "set_model_response",
		description: "Set your final response using the required output schema. " +
			"Use this tool to provide your final structured answer instead of outputting text directly.",
		outputSchema: schemaMap,
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

// Definition returns the tool definition including name, description, and parameters.
func (t *SetModelResponseTool) Definition() *Definition {
	return &Definition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        t.name,
			Description: t.description,
			Parameters:  t.Parameters(),
		},
	}
}

// Parameters returns the tool's parameter schema.
// Extracts properties and required fields from the output schema.
func (t *SetModelResponseTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": t.outputSchema["properties"],
		"required":   t.outputSchema["required"],
	}
}

// Call validates the provided arguments against the schema and returns them.
// The arguments are expected to be a JSON string matching the output schema.
func (t *SetModelResponseTool) Call(ctx context.Context, args string) (any, error) {
	// Validate args against schema
	if err := jsonschema.Validate(t.outputSchema, args); err != nil {
		return nil, fmt.Errorf("tool/set_model_response: invalid model response: %w", err)
	}

	return args, nil
}

// Instruction returns the instruction text to be added to model requests.
// This explains to the model how and when to use the set_model_response tool.
func (t *SetModelResponseTool) Instruction() string {
	return setModelResponseInstruction
}
