package tool

import (
	"context"
	"fmt"
	"maps"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
	"github.com/hupe1980/agentmesh/pkg/schema"
)

const defaultSetModelResponseName = "set_model_response"

const defaultSetModelResponseDescription = "Set your final response using the required output schema. " +
	"Use this tool to provide your final structured answer instead of outputting text directly."

// defaultSetModelResponseInstruction returns the default instruction with the tool name.
func defaultSetModelResponseInstruction(name string) string {
	return fmt.Sprintf("IMPORTANT: You have access to other tools, but you must provide your final "+
		"response using the %s tool with the required structured format. After using any other tools needed "+
		"to complete the task, always call %s with your final answer in the specified schema format.", name, name)
}

// SetModelResponseToolOptions configures the SetModelResponseTool.
type SetModelResponseToolOptions struct {
	// Name overrides the default tool name ("set_model_response").
	Name string
	// Description overrides the default tool description.
	Description string
	// Instruction overrides the default instruction text added to model requests.
	Instruction string
}

// WithSetModelResponseName sets a custom name for the SetModelResponseTool.
func WithSetModelResponseName(name string) func(*SetModelResponseToolOptions) {
	return func(o *SetModelResponseToolOptions) {
		o.Name = name
	}
}

// WithSetModelResponseDescription sets a custom description for the SetModelResponseTool.
func WithSetModelResponseDescription(description string) func(*SetModelResponseToolOptions) {
	return func(o *SetModelResponseToolOptions) {
		o.Description = description
	}
}

// WithInstruction sets a custom instruction text for the SetModelResponseTool.
// This overrides the default instruction that tells the model how to use the tool.
func WithInstruction(instruction string) func(*SetModelResponseToolOptions) {
	return func(o *SetModelResponseToolOptions) {
		o.Instruction = instruction
	}
}

// SetModelResponseTool is an internal tool used when structured output is configured
// alongside other tools. It lets the model provide its final structured response
// using tool calling, enabling structured output on models without native support.
//
// This is the "tool trick" - converting a schema into a tool that the model must
// call to provide its final response.
type SetModelResponseTool struct {
	name         string
	description  string
	instruction  string
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
// Example with custom instruction:
//
//	tool, err := tool.NewSetModelResponseTool(&outputSchema,
//	    tool.WithInstruction("Always use set_model_response for your final answer."),
//	)
//
// The tool automatically:
//   - Validates the response against the schema
//   - Adds instructions to the model request
//   - Returns the validated response for further processing
func NewSetModelResponseTool(outputSchema any, optFns ...func(*SetModelResponseToolOptions)) (*SetModelResponseTool, error) {
	opts := SetModelResponseToolOptions{
		Name:        defaultSetModelResponseName,
		Description: defaultSetModelResponseDescription,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	// Use default instruction with the configured name if not overridden
	instruction := opts.Instruction
	if instruction == "" {
		instruction = defaultSetModelResponseInstruction(opts.Name)
	}

	var schemaMap map[string]any

	switch s := outputSchema.(type) {
	case nil:
		return nil, ErrNilOutputSchema
	case map[string]any:
		schemaMap = maps.Clone(s)
	case schema.OutputSchema:
		schemaMap = maps.Clone(s.Schema)
	case *schema.OutputSchema:
		if s == nil {
			return nil, ErrNilOutputSchemaPointer
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
		name:         opts.Name,
		description:  opts.Description,
		instruction:  instruction,
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
// Returns the custom instruction if configured, otherwise returns the default.
func (t *SetModelResponseTool) Instruction() string {
	return t.instruction
}
