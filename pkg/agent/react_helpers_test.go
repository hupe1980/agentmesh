package agent

import (
	"context"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// TestBuildToolRegistry_Success tests successful tool registry construction.
func TestBuildToolRegistry_Success(t *testing.T) {
	t.Parallel()

	tool1 := &mockTool{name: "calculator"}
	tool2 := &mockTool{name: "search"}
	tool3 := &mockTool{name: "search"} // Duplicate - should be deduplicated

	configTools := []tool.Tool{tool1, tool2, tool3}

	tools, registry := buildToolRegistry(configTools)

	// Should have 2 tools (calculator + search, duplicate removed)
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
	if len(registry) != 2 {
		t.Errorf("expected 2 tools in registry, got %d", len(registry))
	}

	// Verify both tools are present
	if _, ok := registry["calculator"]; !ok {
		t.Error("expected calculator tool in registry")
	}
	if _, ok := registry["search"]; !ok {
		t.Error("expected search tool in registry")
	}
}

// TestBuildToolRegistry_EmptyTools tests with no tools provided.
func TestBuildToolRegistry_EmptyTools(t *testing.T) {
	t.Parallel()

	tools, registry := buildToolRegistry([]tool.Tool{})

	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
	if len(registry) != 0 {
		t.Errorf("expected 0 tools in registry, got %d", len(registry))
	}
}

// TestBuildToolRegistry_NilTools tests with nil tools in the list.
func TestBuildToolRegistry_NilTools(t *testing.T) {
	t.Parallel()

	tool1 := &mockTool{name: "valid"}

	configTools := []tool.Tool{tool1, nil, nil} // nil tools should be skipped

	tools, registry := buildToolRegistry(configTools)

	// Should have 1 tool (nils skipped)
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if len(registry) != 1 {
		t.Errorf("expected 1 tool in registry, got %d", len(registry))
	}
}

// TestValidateModelCapabilities_Success tests successful validation when model supports tools.
func TestValidateModelCapabilities_Success(t *testing.T) {
	t.Parallel()

	mdl := &mockModel{
		capabilities: model.Capabilities{
			Tools: true, // Model supports tools
		},
	}

	tools := []tool.Tool{
		&mockTool{name: "tool1"},
		&mockTool{name: "tool2"},
	}

	err := validateModelCapabilities(mdl, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateModelCapabilities_NoTools tests validation with no tools (should succeed).
func TestValidateModelCapabilities_NoTools(t *testing.T) {
	t.Parallel()

	mdl := &mockModel{
		capabilities: model.Capabilities{
			Tools: false, // Model doesn't support tools, but that's OK if no tools provided
		},
	}

	tools := []tool.Tool{} // No tools

	err := validateModelCapabilities(mdl, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateModelCapabilities_FailsWhenToolsProvidedButNotSupported tests validation failure.
func TestValidateModelCapabilities_FailsWhenToolsProvidedButNotSupported(t *testing.T) {
	t.Parallel()

	mdl := &mockModel{
		capabilities: model.Capabilities{
			Tools: false, // Model DOES NOT support tools
		},
	}

	tools := []tool.Tool{
		&mockTool{name: "unsupported"},
	}

	err := validateModelCapabilities(mdl, tools)
	if err == nil {
		t.Fatal("expected error when tools provided but model doesn't support them")
	}

	// Verify error message contains useful info
	errMsg := err.Error()
	if !containsHelper(errMsg, "does not support tools") {
		t.Error("expected error message to mention tools not supported")
	}
	if !containsHelper(errMsg, "1 tools provided") {
		t.Error("expected error message to mention number of tools provided")
	}
}

// Note: Additional tests for internal helper functions (createModelNode, createToolNode, buildReActGraph)
// would require extensive mocking of internal types and are better covered by integration tests.

// Helper function to check if string contains substring
func containsHelper(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if s == substr {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Mock implementations for testing

type mockTool struct {
	name string
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return "mock tool"
}

func (m *mockTool) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        m.name,
			Description: "mock tool",
			Parameters:  map[string]any{},
		},
	}
}

func (m *mockTool) Call(ctx context.Context, args string) (any, error) {
	return nil, nil
}

type mockModel struct {
	capabilities model.Capabilities
}

func (m *mockModel) Name() string {
	return "mock-model"
}

func (m *mockModel) Capabilities() model.Capabilities {
	return m.capabilities
}

func (m *mockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		// Mock implementation - just return empty response
		yield(&model.Response{}, nil)
	}
}
