package agent

import (
	"context"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// TestBuildCombinedToolset_StaticToolsOnly tests toolset creation with only static tools.
func TestBuildCombinedToolset_StaticToolsOnly(t *testing.T) {
	t.Parallel()

	tool1 := &mockTool{name: "calculator"}
	tool2 := &mockTool{name: "search"}

	staticTools := []tool.Tool{tool1, tool2}
	var toolsets []tool.Toolset

	combinedToolset := buildCombinedToolset(staticTools, toolsets)

	// List tools should return both tools
	tools, err := combinedToolset.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// TestBuildCombinedToolset_ToolsetsOnly tests toolset creation with only dynamic toolsets.
func TestBuildCombinedToolset_ToolsetsOnly(t *testing.T) {
	t.Parallel()

	tool1 := &mockTool{name: "dynamic1"}
	tool2 := &mockTool{name: "dynamic2"}
	mockToolset := tool.NewStaticToolset(tool1, tool2)

	var staticTools []tool.Tool
	toolsets := []tool.Toolset{mockToolset}

	combinedToolset := buildCombinedToolset(staticTools, toolsets)

	// List tools should return both tools from the dynamic toolset
	tools, err := combinedToolset.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// TestBuildCombinedToolset_Mixed tests toolset creation with both static tools and dynamic toolsets.
func TestBuildCombinedToolset_Mixed(t *testing.T) {
	t.Parallel()

	staticTool := &mockTool{name: "static"}
	dynamicTool := &mockTool{name: "dynamic"}
	mockToolset := tool.NewStaticToolset(dynamicTool)

	staticTools := []tool.Tool{staticTool}
	toolsets := []tool.Toolset{mockToolset}

	combinedToolset := buildCombinedToolset(staticTools, toolsets)

	// List tools should return both static and dynamic tools
	tools, err := combinedToolset.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// TestBuildCombinedToolset_Empty tests toolset creation with no tools or toolsets.
func TestBuildCombinedToolset_Empty(t *testing.T) {
	t.Parallel()

	var staticTools []tool.Tool
	var toolsets []tool.Toolset

	combinedToolset := buildCombinedToolset(staticTools, toolsets)

	// List tools should return empty list
	tools, err := combinedToolset.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

// TestBuildCombinedToolset_MultipleToolsets tests combining multiple toolsets.
func TestBuildCombinedToolset_MultipleToolsets(t *testing.T) {
	t.Parallel()

	tool1 := &mockTool{name: "toolset1_tool"}
	tool2 := &mockTool{name: "toolset2_tool"}
	mockToolset1 := tool.NewStaticToolset(tool1)
	mockToolset2 := tool.NewStaticToolset(tool2)

	var staticTools []tool.Tool
	toolsets := []tool.Toolset{mockToolset1, mockToolset2}

	combinedToolset := buildCombinedToolset(staticTools, toolsets)

	// List tools should return tools from both toolsets
	tools, err := combinedToolset.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// Note: Additional tests for internal helper functions are better covered by integration tests.

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
