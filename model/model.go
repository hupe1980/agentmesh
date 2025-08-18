package model

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
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
	Stream       bool             `json:"stream,omitempty"`
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

// MockModel is a lightweight in‑memory Model useful for tests & examples.
type MockModel struct {
	info      Info
	responses map[string]string
}

// NewMockModel constructs a MockModel with basic tool support enabled.
func NewMockModel(name, provider string) *MockModel {
	return &MockModel{
		info: Info{
			Name:          name,
			Provider:      provider,
			SupportsTools: true,
		},
		responses: make(map[string]string),
	}
}

// AddResponse registers a deterministic canned completion for an input prompt.
func (m *MockModel) AddResponse(prompt, response string) { m.responses[prompt] = response }

// Generate implements Model; emits optional streaming char chunks then final response.
func (m *MockModel) Generate(ctx context.Context, req Request) (<-chan Response, <-chan error) {
	respCh := make(chan Response, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(respCh)
		defer close(errCh)
		if len(req.Contents) == 0 {
			errCh <- fmt.Errorf("no contents provided")
			return
		}
		last := req.Contents[len(req.Contents)-1]
		var inputText string
		for _, p := range last.Parts {
			if tp, ok := p.(core.TextPart); ok {
				inputText += tp.Text
			}
		}
		full := m.responses[inputText]
		if full == "" {
			full = fmt.Sprintf("Mock response to: %s", inputText)
		}
		if req.Stream {
			for _, r := range full {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case respCh <- Response{
					Partial: true,
					Content: core.Content{
						Role:  "assistant",
						Parts: []core.Part{core.TextPart{Text: string(r)}},
					},
				}:
				}
			}
		}
		respCh <- Response{
			Partial: false,
			Content: core.Content{
				Role:  "assistant",
				Parts: []core.Part{core.TextPart{Text: full}},
			},
			FinishReason: "stop",
		}
	}()
	return respCh, errCh
}

// Info implements Model interface.
func (m *MockModel) Info() Info { return m.info }
