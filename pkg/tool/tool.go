package tool

import (
	"context"
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

// Toolset defines a collection of tools that can be managed together.
type Toolset interface {
	ListTools(ctx context.Context) ([]Tool, error)
	Close() error
}
