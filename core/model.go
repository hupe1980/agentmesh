package core

import (
	"context"
	"encoding/json"
	"strings"
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
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// Message represents a message in a conversation, consisting of a role and parts.
type Message struct {
	Role  Role   `json:"role,omitempty"` // Conversation role (user, assistant, tool, system,...)
	Parts []Part `json:"parts"`          // Ordered heterogeneous parts
}

// ModelRequest captures the normalized model input produced by flows.
type ModelRequest struct {
	Instructions string           `json:"instructions"` // Instructions for the model
	Messages     []*Message       `json:"messages"`     // Conversation messages (role + parts)
	Tools        []ToolDefinition `json:"tools,omitempty"`
	Stream       bool             `json:"stream,omitempty"`

	// ToolRegistry holds the runtime tool implementations keyed by name (not serialized)
	ToolRegistry map[string]Tool `json:"-"`
}

// AppendInstructions appends one or more instruction fragments to the request,
// joining them with a blank line ("\n\n"). Empty strings are ignored.
func (r *ModelRequest) AppendInstructions(parts ...string) {
	// filter empties
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return
	}

	joined := strings.Join(filtered, "\n\n")
	if r.Instructions == "" {
		r.Instructions = joined
		return
	}

	r.Instructions = r.Instructions + "\n\n" + joined
}

// AddTool registers a single tool with the request, updating both the serialized
// tool definition slice (for the model provider) and the internal ToolRegistry map
// used at execution time. Duplicate names overwrite previous entries.
func (r *ModelRequest) AddTool(t Tool) {
	if r.ToolRegistry == nil {
		r.ToolRegistry = make(map[string]Tool)
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

	r.ToolRegistry[t.Name()] = t
}

// AddTools registers multiple tools convenience wrapper.
func (r *ModelRequest) AddTools(ts ...Tool) {
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

// ModelResponse is a (partial or final) chunk emitted by a streaming model.
type ModelResponse struct {
	ID           string      `json:"id"`
	Partial      bool        `json:"partial"` // Indicates if this is a partial response
	Parts        []Part      `json:"parts,omitempty"`
	FinishReason string      `json:"finish_reason"` // "stop", "length", "tool_calls", etc.
	Usage        *TokenUsage `json:"usage,omitempty"`
}

// Model is the minimal interface required by flows & agents to drive generation.
type Model interface {
	// Generate initiates a generation request to the model.
	Generate(ctx context.Context, req *ModelRequest) (<-chan *ModelResponse, <-chan error)
}

// ModelExecutor abstracts execution of a Model request, allowing decoration
// (metrics, tracing, timeouts) similar to AgentExecutor / ToolExecutor.
type ModelExecutor interface {
	Execute(
		ctx context.Context,
		reqCtx RequestContext,
		model Model,
		req *ModelRequest,
	) (<-chan *ModelResponse, <-chan error)
}

// ModelExecutorFunc is an adapter to allow plain functions to satisfy ModelExecutor.
type ModelExecutorFunc func(context.Context, RequestContext, Model, *ModelRequest) (<-chan *ModelResponse, <-chan error)

// Execute calls the underlying function to execute the model request.
func (f ModelExecutorFunc) Execute(
	ctx context.Context,
	reqCtx RequestContext,
	model Model,
	req *ModelRequest,
) (<-chan *ModelResponse, <-chan error) {
	return f(ctx, reqCtx, model, req)
}
