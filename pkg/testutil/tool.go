package testutil

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

// MockTool is a configurable mock implementation of tool.Tool.
// Can be used directly with struct literal syntax for backward compatibility:
//
//	tool := &testutil.MockTool{
//	    NameValue:        "my_tool",
//	    DescriptionValue: "Does something",
//	    CallFunc: func(ctx context.Context, args string) (any, error) { ... },
//	}
//
// Or use the builder pattern for more complex scenarios:
//
//	tool := testutil.NewToolBuilder("my_tool").WithResult("ok").Build()
type MockTool struct {
	// NameValue is the tool name. Can be set directly for simple mocks.
	NameValue string
	// DescriptionValue is the tool description. Can be set directly for simple mocks.
	DescriptionValue string
	// CallFunc is the function called by Call. Can be set directly for simple mocks.
	CallFunc func(ctx context.Context, args string) (any, error)
	// SchemaValue is the tool's definition. Can be set directly for simple mocks.
	SchemaValue *tool.Definition

	invocations []ToolInvocation
	mu          sync.Mutex
}

// ToolInvocation records a single tool invocation.
type ToolInvocation struct {
	Input  string
	Output any
	Error  error
}

// Name returns the tool name.
func (t *MockTool) Name() string {
	if t.NameValue == "" {
		return "mock_tool"
	}
	return t.NameValue
}

// Description returns the tool description.
func (t *MockTool) Description() string {
	if t.DescriptionValue == "" {
		return "A mock tool for testing"
	}
	return t.DescriptionValue
}

// Definition returns the tool's definition.
func (t *MockTool) Definition() *tool.Definition {
	if t.SchemaValue != nil {
		return t.SchemaValue
	}
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "Input parameter",
					},
				},
			},
		},
	}
}

// Call runs the tool with the given input.
func (t *MockTool) Call(ctx context.Context, args string) (any, error) {
	var result any
	var err error

	if t.CallFunc != nil {
		result, err = t.CallFunc(ctx, args)
	} else {
		result = "mock result"
	}

	// Record invocation
	t.mu.Lock()
	t.invocations = append(t.invocations, ToolInvocation{
		Input:  args,
		Output: result,
		Error:  err,
	})
	t.mu.Unlock()

	return result, err
}

// Invocations returns all recorded invocations.
func (t *MockTool) Invocations() []ToolInvocation {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]ToolInvocation{}, t.invocations...)
}

// CallCount returns the number of times the tool was invoked.
func (t *MockTool) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.invocations)
}

// LastInput returns the input from the last invocation.
func (t *MockTool) LastInput() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.invocations) == 0 {
		return ""
	}
	return t.invocations[len(t.invocations)-1].Input
}

// Reset clears all recorded invocations.
func (t *MockTool) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.invocations = nil
}

// ToolBuilder provides a fluent API for building MockTool instances.
type ToolBuilder struct {
	name        string
	description string
	definition  *tool.Definition
	callFunc    func(ctx context.Context, args string) (any, error)
	results     []string
	errors      []error
}

// NewToolBuilder creates a new ToolBuilder with the specified name.
func NewToolBuilder(name string) *ToolBuilder {
	return &ToolBuilder{
		name:        name,
		description: "Mock tool: " + name,
	}
}

// WithDescription sets the tool description.
func (b *ToolBuilder) WithDescription(desc string) *ToolBuilder {
	b.description = desc
	return b
}

// WithDefinition sets the tool's definition.
func (b *ToolBuilder) WithDefinition(def *tool.Definition) *ToolBuilder {
	b.definition = def
	return b
}

// WithResult sets a static result for the tool.
func (b *ToolBuilder) WithResult(result string) *ToolBuilder {
	b.results = append(b.results, result)
	return b
}

// WithResults sets multiple sequential results.
func (b *ToolBuilder) WithResults(results ...string) *ToolBuilder {
	b.results = append(b.results, results...)
	return b
}

// WithError sets an error to return.
func (b *ToolBuilder) WithError(err error) *ToolBuilder {
	b.errors = append(b.errors, err)
	return b
}

// WithCall sets a custom call function.
func (b *ToolBuilder) WithCall(fn func(ctx context.Context, args string) (any, error)) *ToolBuilder {
	b.callFunc = fn
	return b
}

// WithJSONHandler sets a call function that unmarshals input to a typed struct.
func WithJSONHandler[T any](handler func(ctx context.Context, input T) (any, error)) func(ctx context.Context, args string) (any, error) {
	return func(ctx context.Context, args string) (any, error) {
		var parsed T
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			return nil, err
		}
		return handler(ctx, parsed)
	}
}

// Build creates the MockTool with the configured settings.
func (b *ToolBuilder) Build() *MockTool {
	callFunc := b.callFunc

	// If no custom call function but results/errors are set
	if callFunc == nil && (len(b.results) > 0 || len(b.errors) > 0) {
		callFunc = b.buildSequentialCallFunc()
	}

	return &MockTool{
		NameValue:        b.name,
		DescriptionValue: b.description,
		SchemaValue:      b.definition,
		CallFunc:         callFunc,
	}
}

// buildSequentialCallFunc creates a call function that returns results/errors sequentially.
func (b *ToolBuilder) buildSequentialCallFunc() func(ctx context.Context, args string) (any, error) {
	idx := 0
	var mu sync.Mutex

	return func(ctx context.Context, args string) (any, error) {
		mu.Lock()
		currentIdx := idx
		idx++
		mu.Unlock()

		// Check for errors first
		if currentIdx < len(b.errors) && b.errors[currentIdx] != nil {
			return nil, b.errors[currentIdx]
		}

		// Return result
		if len(b.results) > 0 {
			resultIdx := min(currentIdx, len(b.results)-1)
			return b.results[resultIdx], nil
		}

		return "mock result", nil
	}
}

// Ensure MockTool implements tool.Tool
var _ tool.Tool = (*MockTool)(nil)
