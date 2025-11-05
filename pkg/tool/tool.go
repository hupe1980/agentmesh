package tool

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// FunctionDefinition describes an individual function (tool) exposed to the model.
// Parameters is a JSON Schema object (draft agnostic, minimal subset expected).
type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// ToolDefinition declaratively exposes a callable function to the model.
type ToolDefinition struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// Tool defines the interface for executable functions that can be called by LLMs.
type Tool interface {
	Name() string
	Description() string
	Definition() *ToolDefinition
	Call(ctx context.Context, args string) (any, error)
}

// Toolset defines a collection of tools that can be managed together.
type Toolset interface {
	ListTools(ctx context.Context, s graph.StateReader) ([]Tool, error)
	Close() error
}
