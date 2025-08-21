package model

import (
	"context"
	"encoding/json"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/tool"
)

// ToolCall represents a function call request surfaced by a model provider.
// Unified across vendors so downstream logic does not need per-provider branching.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction describes the concrete function target of a tool call.
type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // JSON string of arguments
}

// ToolDefinition declaratively exposes a callable function to the model.
type ToolDefinition struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes an individual function (tool) exposed to the model.
// Parameters is a JSON Schema object (draft agnostic, minimal subset expected).
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// Request captures the normalized model input produced by flows.
type Request struct {
	Instructions string           `json:"instructions"` // Instructions for the model
	Contents     []core.Content   `json:"contents"`     // Higher-level content converted to provider messages
	Tools        []ToolDefinition `json:"tools,omitempty"`
	// RawTools holds the runtime tool implementations keyed by name (not serialized)
	RawTools map[string]tool.Tool `json:"-"`
	Stream   bool                 `json:"stream,omitempty"`
}

// AddTool registers a single tool with the request, updating both the serialized
// tool definition slice (for the model provider) and the internal RawTools map
// used at execution time. Duplicate names overwrite previous entries.
func (r *Request) AddTool(t tool.Tool) {
	if r.RawTools == nil {
		r.RawTools = make(map[string]tool.Tool)
	}
	// Append / replace definition
	def := ToolDefinition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		},
	}
	// Replace existing definition if same name
	replaced := false
	for i, existing := range r.Tools {
		if existing.Function.Name == def.Function.Name {
			r.Tools[i] = def
			replaced = true
			break
		}
	}
	if !replaced {
		r.Tools = append(r.Tools, def)
	}
	r.RawTools[t.Name()] = t
}

// AddTools registers multiple tools convenience wrapper.
func (r *Request) AddTools(ts ...tool.Tool) {
	for _, t := range ts {
		r.AddTool(t)
	}
}

// TokenUsage captures token usage statistics for a response.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response is a (partial or final) chunk emitted by a streaming model.
type Response struct {
	ID           string       `json:"id"`
	Partial      bool         `json:"partial"` // Indicates if this is a partial response
	Content      core.Content `json:"content"`
	FinishReason string       `json:"finish_reason"` // "stop", "length", "tool_calls", etc.
	Usage        *TokenUsage  `json:"usage,omitempty"`
}

// Info contains metadata about a model implementation.
// Previously named ModelInfo; renamed to avoid stutter at call sites.
type Info struct {
	Name          string `json:"name"`
	Provider      string `json:"provider"` // "openai", "anthropic", "local", etc.
	SupportsTools bool   `json:"supports_tools"`
}

// Model is the minimal interface required by flows & agents to drive generation.
type Model interface {
	Generate(ctx context.Context, req Request) (<-chan Response, <-chan error)

	// Info returns information about the model implementation.
	Info() Info
}
