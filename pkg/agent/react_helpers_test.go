package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// TestBuildCombinedToolset_StaticToolsOnly tests toolset creation with only static tools.
func TestBuildCombinedToolset_StaticToolsOnly(t *testing.T) {
	t.Parallel()

	tool1 := testutil.NewToolBuilder("calculator").Build()
	tool2 := testutil.NewToolBuilder("search").Build()

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

	tool1 := testutil.NewToolBuilder("dynamic1").Build()
	tool2 := testutil.NewToolBuilder("dynamic2").Build()
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

	staticTool := testutil.NewToolBuilder("static").Build()
	dynamicTool := testutil.NewToolBuilder("dynamic").Build()
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

	tool1 := testutil.NewToolBuilder("toolset1_tool").Build()
	tool2 := testutil.NewToolBuilder("toolset2_tool").Build()
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
