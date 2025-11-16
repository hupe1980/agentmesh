package compile

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidation_BasicStructure(t *testing.T) {
	t.Run("empty graph with default options", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		// Empty graph is allowed by default
		assert.Empty(t, errors)
	})

	t.Run("empty graph with strict validation", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		validator := NewValidator(StrictValidationOptions())
		errors := validator.Validate(g)

		// Strict mode requires nodes
		assert.NotEmpty(t, errors)
		assert.Equal(t, ErrTypeEmptyGraph, errors[0].Type)
	})

	t.Run("node with nil RunFunc", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name:    "invalid",
			RunFunc: nil,
		})

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeInvalidNode && err.Node == "invalid" {
				found = true
				assert.Contains(t, err.Message, "nil RunFunc")
			}
		}
		assert.True(t, found, "should detect nil RunFunc")
	})

	t.Run("node with reserved name", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: StartNode,
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeInvalidNode {
				found = true
				assert.Contains(t, err.Message, "reserved name")
			}
		}
		assert.True(t, found, "should detect reserved name")
	})
}

func TestValidation_Edges(t *testing.T) {
	t.Run("edge to non-existent node", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "a",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge("a", "non_existent")

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeMissingNode {
				found = true
				assert.Contains(t, err.Message, "non_existent")
			}
		}
		assert.True(t, found, "should detect missing target node")
	})

	t.Run("edge from non-existent node", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "b",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge("non_existent", "b")

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeMissingNode {
				found = true
				assert.Contains(t, err.Message, "non_existent")
			}
		}
		assert.True(t, found, "should detect missing source node")
	})

	t.Run("edge from END node", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "a",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge(EndNode, "a")

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeInvalidEdge {
				found = true
				assert.Contains(t, err.Message, "from END node")
			}
		}
		assert.True(t, found, "should detect edge from END")
	})

	t.Run("edge to START node", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "a",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge("a", StartNode)

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeInvalidEdge {
				found = true
				assert.Contains(t, err.Message, "to START node")
			}
		}
		assert.True(t, found, "should detect edge to START")
	})
}

func TestValidation_Conditionals(t *testing.T) {
	t.Run("conditional from non-existent node", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddConditionalEdges("non_existent", func(ctx context.Context, s state.Reader) []string {
			return []string{"a"}
		}, []string{"a"})

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeMissingNode && err.Node == "non_existent" {
				found = true
			}
		}
		assert.True(t, found, "should detect missing source node")
	})

	t.Run("conditional to non-existent node", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "router",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddConditionalEdges("router", func(ctx context.Context, s state.Reader) []string {
			return []string{"non_existent"}
		}, []string{"non_existent"})

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeMissingNode {
				found = true
				assert.Contains(t, err.Message, "non_existent")
			}
		}
		assert.True(t, found, "should detect missing target node")
	})

	t.Run("conditional with nil condition function", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "router",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddNode(&graph.Node{
			Name: "target",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddConditionalEdges("router", nil, []string{"target"})

		validator := NewValidator(DefaultValidationOptions())
		errors := validator.Validate(g)

		assert.NotEmpty(t, errors)
		found := false
		for _, err := range errors {
			if err.Type == ErrTypeInvalidCondition {
				found = true
				assert.Contains(t, err.Message, "nil condition")
			}
		}
		assert.True(t, found, "should detect nil condition function")
	})
}

func TestValidation_Topology(t *testing.T) {
	t.Run("cycle detection", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "a",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddNode(&graph.Node{
			Name: "b",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge("a", "b")
		g.AddEdge("b", "a") // Creates cycle

		// Default options allow cycles
		defaultValidator := NewValidator(DefaultValidationOptions())
		defaultErrors := defaultValidator.Validate(g)
		assert.Empty(t, defaultErrors, "default validation should allow cycles")

		// Strict validation detects cycles
		strictOpts := StrictValidationOptions()
		strictValidator := NewValidator(strictOpts)
		strictErrors := strictValidator.Validate(g)

		found := false
		for _, err := range strictErrors {
			if err.Type == ErrTypeCycle {
				found = true
				assert.Contains(t, err.Message, "cycle detected")
			}
		}
		assert.True(t, found, "strict validation should detect cycle")
	})

	t.Run("unreachable node detection", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "reachable",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddNode(&graph.Node{
			Name: "unreachable",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge(StartNode, "reachable")
		g.AddEdge("reachable", EndNode)
		// "unreachable" has no incoming edges

		// Default options allow unreachable nodes
		defaultValidator := NewValidator(DefaultValidationOptions())
		defaultErrors := defaultValidator.Validate(g)
		assert.Empty(t, defaultErrors, "default validation should allow unreachable nodes")

		// Strict validation detects unreachable nodes
		strictOpts := StrictValidationOptions()
		strictValidator := NewValidator(strictOpts)
		strictErrors := strictValidator.Validate(g)

		found := false
		for _, err := range strictErrors {
			if err.Type == ErrTypeUnreachableNode && err.Node == "unreachable" {
				found = true
			}
		}
		assert.True(t, found, "strict validation should detect unreachable node")
	})

	t.Run("dead end node detection", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "dead_end",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge(StartNode, "dead_end")
		// "dead_end" has no outgoing edges to END

		// Default options allow dead ends
		defaultValidator := NewValidator(DefaultValidationOptions())
		defaultErrors := defaultValidator.Validate(g)
		assert.Empty(t, defaultErrors, "default validation should allow dead ends")

		// Strict validation detects dead ends
		strictOpts := StrictValidationOptions()
		strictValidator := NewValidator(strictOpts)
		strictErrors := strictValidator.Validate(g)

		found := false
		for _, err := range strictErrors {
			if err.Type == ErrTypeDeadEnd && err.Node == "dead_end" {
				found = true
			}
		}
		assert.True(t, found, "strict validation should detect dead end node")
	})
}

func TestCompile_WithValidation(t *testing.T) {
	t.Run("valid graph compiles successfully", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "process",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge(StartNode, "process")
		g.AddEdge("process", EndNode)

		compiled, err := Compile(g, stateManager)
		require.NoError(t, err)
		require.NotNil(t, compiled)
	})

	t.Run("invalid graph fails compilation", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "a",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge("a", "non_existent")

		_, err := Compile(g, stateManager)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
		assert.Contains(t, err.Error(), "non_existent")
	})

	t.Run("strict validation enforces constraints", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "orphan",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		// No edges - orphaned node

		// Default validation passes
		_, err := Compile(g, stateManager)
		require.NoError(t, err)

		// Strict validation fails
		_, err = Compile(g, stateManager, WithStrictValidation())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("disable validation", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		// Intentionally invalid graph
		g.AddNode(&graph.Node{
			Name: "a",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddEdge("a", "non_existent")

		// With validation disabled, compilation should succeed
		compiled, err := Compile(g, stateManager, WithoutValidation())
		require.NoError(t, err)
		require.NotNil(t, compiled)
	})
}

func TestValidation_ComplexGraphs(t *testing.T) {
	t.Run("diamond pattern is valid", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		// Diamond: start -> split -> (left, right) -> merge -> end
		for _, name := range []string{"split", "left", "right", "merge"} {
			g.AddNode(&graph.Node{
				Name: name,
				RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
					return &graph.NodeResult{}, nil
				},
			})
		}

		g.AddEdge(StartNode, "split")
		g.AddEdge("split", "left")
		g.AddEdge("split", "right")
		g.AddEdge("left", "merge")
		g.AddEdge("right", "merge")
		g.AddEdge("merge", EndNode)

		validator := NewValidator(StrictValidationOptions())
		errors := validator.Validate(g)
		assert.Empty(t, errors, "diamond pattern should be valid")
	})

	t.Run("conditional branches are valid", func(t *testing.T) {
		stateManager, _ := state.NewStateManager(0)
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.Node{
			Name: "router",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddNode(&graph.Node{
			Name: "pathA",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})
		g.AddNode(&graph.Node{
			Name: "pathB",
			RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})

		g.AddEdge(StartNode, "router")
		g.AddConditionalEdges("router", func(ctx context.Context, s state.Reader) []string {
			return []string{"pathA"}
		}, []string{"pathA", "pathB"})
		g.AddEdge("pathA", EndNode)
		g.AddEdge("pathB", EndNode)

		validator := NewValidator(StrictValidationOptions())
		errors := validator.Validate(g)
		assert.Empty(t, errors, "conditional branches should be valid")
	})
}
