package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ====================
// Basic Validation Tests
// ====================

func TestValidateNoEntryPoint(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	// No Start() call

	errs := g.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error for missing entry point")
	}
	if errs[0].Type != graph.ErrorTypeInvalidEntryNode {
		t.Errorf("expected ErrorTypeInvalidEntryNode, got %v", errs[0].Type)
	}
}

func TestValidateInvalidEntryPoint(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("nonexistent")

	errs := g.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid entry point")
	}
	if errs[0].Type != graph.ErrorTypeInvalidEntryNode {
		t.Errorf("expected ErrorTypeInvalidEntryNode, got %v", errs[0].Type)
	}
}

func TestValidateInvalidEdge(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("nonexistent")
	}, "nonexistent") // Target doesn't exist
	g.Start("a")

	errs := g.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid edge")
	}
	if errs[0].Type != graph.ErrorTypeInvalidEdge {
		t.Errorf("expected ErrorTypeInvalidEdge, got %v", errs[0].Type)
	}
}

func TestValidateValidGraph(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("b")
	}, "b")
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	errs := g.Validate()
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

// ====================
// Strict Validation Tests
// ====================

func TestValidateCycleDetection(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("b")
	}, "b")
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("a") // Cycle back to a
	}, "a")
	g.Start("a")

	// Default validation allows cycles
	errs := g.Validate()
	if len(errs) != 0 {
		t.Errorf("default validation should allow cycles, got %v", errs)
	}

	// Strict validation detects cycles
	errs = g.Validate(graph.StrictValidationOptions())
	if len(errs) == 0 {
		t.Fatal("strict validation should detect cycle")
	}
	if errs[0].Type != graph.ErrorTypeCycle {
		t.Errorf("expected ErrorTypeCycle, got %v", errs[0].Type)
	}
}

func TestValidateDisconnectedNode(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END) // Not reachable from "a"
	}, graph.END)
	g.Start("a")

	// Default (basic) validation does NOT detect disconnected nodes
	errs := g.Validate()
	if len(errs) != 0 {
		t.Fatalf("basic validation should not detect disconnected nodes, got %v", errs)
	}

	// Strict validation detects disconnected nodes
	errs = g.Validate(graph.StrictValidationOptions())
	if len(errs) == 0 {
		t.Fatal("strict validation should detect disconnected node")
	}

	// Find disconnected error
	found := false
	for _, err := range errs {
		if err.Type == graph.ErrorTypeDisconnected && err.Node == "b" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected disconnected error for node 'b', got %v", errs)
	}
}

func TestValidateAllowDisconnected(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END) // Not reachable from "a"
	}, graph.END)
	g.Start("a")

	// Custom options allowing disconnected nodes
	opts := graph.ValidationOptions{
		Level:                  graph.ValidationLevelStrict,
		AllowCycles:            true,
		AllowDisconnectedNodes: true,
	}
	errs := g.Validate(opts)
	if len(errs) != 0 {
		t.Errorf("expected no errors with AllowDisconnectedNodes, got %v", errs)
	}
}

// ====================
// Build with Validation Options Tests
// ====================

func TestBuildWithStrictValidation(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("b")
	}, "b")
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("a") // Cycle
	}, "a")
	g.Start("a")

	// Default build allows cycles
	_, err := g.Build()
	if err != nil {
		t.Errorf("default build should allow cycles, got %v", err)
	}
}

func TestBuildWithStrictValidationRejectsCycle(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("b")
	}, "b")
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("a") // Cycle
	}, "a")
	g.Start("a")

	// Strict build rejects cycles
	_, err := g.Build(graph.WithStrictValidation())
	if err == nil {
		t.Fatal("strict build should reject cycle")
	}
}

func TestBuildWithoutValidation(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("nonexistent") // Invalid edge
	}, "nonexistent")
	g.Start("a")

	// Build without validation should succeed even with invalid edges
	_, err := g.Build(graph.WithoutValidation())
	if err != nil {
		t.Errorf("build without validation should succeed, got %v", err)
	}
}

func TestValidateNoNodes(t *testing.T) {
	g := graph.New()
	g.Start("a") // Entry point but no nodes

	errs := g.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation errors for empty graph")
	}
}

// ====================
// Complex Graph Validation Tests
// ====================

func TestValidateComplexGraph(t *testing.T) {
	g := graph.New()
	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("a", "b") // Fan-out
	}, "a", "b")
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("merge")
	}, "merge")
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("merge")
	}, "merge")
	g.Node("merge", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END) // Fan-in
	}, graph.END)
	g.Start("start")

	errs := g.Validate(graph.StrictValidationOptions())
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid complex graph, got %v", errs)
	}
}

func TestValidateParallelEntryPoints(t *testing.T) {
	g := graph.New()
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("merge")
	}, "merge")
	g.Node("b", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("merge")
	}, "merge")
	g.Node("merge", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a", "b") // Multiple entry points

	errs := g.Validate(graph.StrictValidationOptions())
	if len(errs) != 0 {
		t.Errorf("expected no errors for parallel entry points, got %v", errs)
	}
}

func TestValidateDuplicateKey(t *testing.T) {
	key1 := graph.NewKey[string]("mykey")
	key2 := graph.NewKey[int]("mykey") // Same name, different type

	g := graph.New(key1, key2)
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	errs := g.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error for duplicate key")
	}
	if errs[0].Type != graph.ErrorTypeDuplicateKey {
		t.Errorf("expected ErrorTypeDuplicateKey, got %v", errs[0].Type)
	}
}

func TestBuildWithDuplicateKey(t *testing.T) {
	key1 := graph.NewKey[string]("mykey")
	key2 := graph.NewKey[int]("mykey") // Same name, different type

	g := graph.New(key1, key2)
	g.Node("a", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")

	_, err := g.Build()
	if err == nil {
		t.Fatal("expected build error for duplicate key")
	}
}
