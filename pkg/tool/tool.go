package tool

import (
	"context"
	"strings"
)

// FunctionDefinition describes an individual function (tool) exposed to the model.
// Parameters is a JSON Schema object (draft agnostic, minimal subset expected).
type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// Definition declaratively exposes a callable function to the model.
type Definition struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// Tool defines the interface for executable functions that can be called by LLMs.
type Tool interface {
	Name() string
	Description() string
	Definition() *Definition
	Call(ctx context.Context, args string) (any, error)
}

// InstructionProvider is an optional interface that tools can implement
// to provide additional instructions that should be appended to the model's
// system prompt. This is useful for tools that need to explain special
// usage patterns to the model.
//
// Example use cases:
//   - SetModelResponseTool: Instructs the model to use this tool for final responses
//   - Search tools: Provide query formatting guidelines
//   - API tools: Explain rate limits or authentication requirements
type InstructionProvider interface {
	// Instruction returns additional instruction text to append to the system prompt.
	// Return an empty string if no additional instructions are needed.
	Instruction() string
}

// CollectInstructions gathers instructions from all tools that implement InstructionProvider.
// Returns a combined string with all instructions separated by double newlines.
// Returns empty string if no tools provide instructions.
func CollectInstructions(tools []Tool) string {
	var instructions []string

	for _, t := range tools {
		if ip, ok := t.(InstructionProvider); ok {
			if instr := ip.Instruction(); instr != "" {
				instructions = append(instructions, instr)
			}
		}
	}

	if len(instructions) == 0 {
		return ""
	}

	return strings.Join(instructions, "\n\n")
}
