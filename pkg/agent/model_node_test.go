package agent

import (
	"context"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewModelNodeFunc(t *testing.T) {
	t.Run("creates node function successfully", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("test response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl)
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("returns error for nil model", func(t *testing.T) {
		_, err := NewModelNodeFunc(nil)
		require.Error(t, err)
	})

	t.Run("with instructions", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithModelInstructions("You are helpful"))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("with output schema", func(t *testing.T) {
		outputSchema := &schema.OutputSchema{
			Name:        "TestSchema",
			Description: "Test schema",
			Schema: map[string]any{
				"type": "object",
			},
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("structured response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithModelOutputSchema(outputSchema))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("with tools", func(t *testing.T) {
		tool1 := &testutil.MockTool{NameValue: "tool1"}
		tool2 := &testutil.MockTool{NameValue: "tool2"}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithModelTools(tool1, tool2))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("with custom tool target", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithToolTarget("custom_tool_node"))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("with custom next target", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithNextTarget("next_node"))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("with response key", func(t *testing.T) {
		responseKey := graph.NewKey[message.Message]("model_response")

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("test response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithModelResponseKey(responseKey))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})
}

func TestModelNodeFunc_Execution(t *testing.T) {
	t.Run("executes and returns command", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("AI response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl)
		require.NoError(t, err)

		// Create a view with messages
		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		cmd, err := nodeFn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		// Should route to END (no tool calls)
		assert.Contains(t, cmd.Next, graph.END)
	})

	t.Run("routes to tool node when tool calls present", func(t *testing.T) {
		// Create an AI message with tool calls
		aiMsg := message.NewAIMessageFromText("")
		aiMsg.ToolCalls = []message.ToolCall{
			{ID: "call1", Name: "test_tool", Arguments: "{}"},
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return aiMsg, nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl)
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Use a tool"),
			},
		})

		cmd, err := nodeFn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		// Should route to tool node
		assert.Contains(t, cmd.Next, "tool")
	})

	t.Run("stores response in state key when configured", func(t *testing.T) {
		responseKey := graph.NewKey[message.Message]("model_response")

		expectedMsg := message.NewAIMessageFromText("AI response")
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return expectedMsg, nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithModelResponseKey(responseKey))
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		cmd, err := nodeFn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		// Verify the response key was set in the command
		require.NotNil(t, cmd.Updates)
		require.Len(t, cmd.Updates, 2) // messages + model_response
		storedMsg, ok := cmd.Updates[responseKey.Name()]
		require.True(t, ok)
		assert.Equal(t, expectedMsg, storedMsg)
		// Verify messages are also added
		_, hasMessages := cmd.Updates[graph.MessagesKeyName]
		assert.True(t, hasMessages)
	})

	t.Run("does not store response when key not configured", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("AI response"), nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl)
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		cmd, err := nodeFn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		// Should only have messages in updates (no custom response key)
		require.NotNil(t, cmd.Updates)
		assert.Len(t, cmd.Updates, 1)
		_, hasMessages := cmd.Updates[graph.MessagesKeyName]
		assert.True(t, hasMessages)
	})

	t.Run("does not store response when tool calls present", func(t *testing.T) {
		responseKey := graph.NewKey[message.Message]("model_response")

		aiMsg := message.NewAIMessageFromText("")
		aiMsg.ToolCalls = []message.ToolCall{
			{ID: "call1", Name: "test_tool", Arguments: "{}"},
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return aiMsg, nil
			}),
		}

		nodeFn, err := NewModelNodeFunc(mdl, WithModelResponseKey(responseKey))
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Use a tool"),
			},
		})

		cmd, err := nodeFn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		// Should route to tool node and NOT store response (tool calls present)
		assert.Contains(t, cmd.Next, "tool")
		// Should only have messages in updates (no custom response key)
		require.NotNil(t, cmd.Updates)
		assert.Len(t, cmd.Updates, 1)
		_, hasMessages := cmd.Updates[graph.MessagesKeyName]
		assert.True(t, hasMessages)
	})
}

// createTestScope creates a Scope for testing using BSPState
func createTestScope(data map[string]any) *testutil.TestScope {
	return testutil.NewTestScopeFromMap(data)
}

// mockToolWithInstruction implements tool.Tool and tool.InstructionProvider
type mockToolWithInstruction struct {
	name        string
	instruction string
}

func (m *mockToolWithInstruction) Name() string {
	return m.name
}

func (m *mockToolWithInstruction) Description() string {
	return "A mock tool with instruction"
}

func (m *mockToolWithInstruction) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        m.name,
			Description: "A mock tool with instruction",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (m *mockToolWithInstruction) Call(_ context.Context, _ string) (any, error) {
	return "result", nil
}

func (m *mockToolWithInstruction) Instruction() string {
	return m.instruction
}

func TestModelNodeFunc_ToolInstructionMerging(t *testing.T) {
	t.Run("merges tool instructions with system prompt", func(t *testing.T) {
		var capturedInstructions string

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				capturedInstructions = req.Instructions
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("response"),
						Partial: false,
					}, nil)
				}
			},
		}

		toolWithInstruction := &mockToolWithInstruction{
			name:        "special_tool",
			instruction: "Always use this tool for special tasks.",
		}

		nodeFn, err := NewModelNodeFunc(mdl,
			WithModelInstructions("You are a helpful assistant."),
			WithModelTools(toolWithInstruction),
		)
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		_, err = nodeFn(context.Background(), scope)
		require.NoError(t, err)

		// Verify instructions contains both base prompt and tool instruction
		assert.Contains(t, capturedInstructions, "You are a helpful assistant.")
		assert.Contains(t, capturedInstructions, "Always use this tool for special tasks.")
	})

	t.Run("uses only tool instructions when no base instructions", func(t *testing.T) {
		var capturedInstructions string

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				capturedInstructions = req.Instructions
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("response"),
						Partial: false,
					}, nil)
				}
			},
		}

		toolWithInstruction := &mockToolWithInstruction{
			name:        "special_tool",
			instruction: "Tool instruction only.",
		}

		nodeFn, err := NewModelNodeFunc(mdl,
			WithModelTools(toolWithInstruction),
		)
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		_, err = nodeFn(context.Background(), scope)
		require.NoError(t, err)

		// Verify instructions is the tool instruction
		assert.Equal(t, "Tool instruction only.", capturedInstructions)
	})

	t.Run("uses only base instructions when no tool instructions", func(t *testing.T) {
		var capturedInstructions string

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				capturedInstructions = req.Instructions
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("response"),
						Partial: false,
					}, nil)
				}
			},
		}

		// MockTool doesn't implement InstructionProvider
		regularTool := &testutil.MockTool{NameValue: "regular_tool"}

		nodeFn, err := NewModelNodeFunc(mdl,
			WithModelInstructions("Base system prompt."),
			WithModelTools(regularTool),
		)
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		_, err = nodeFn(context.Background(), scope)
		require.NoError(t, err)

		// Verify instructions is just the base prompt
		assert.Equal(t, "Base system prompt.", capturedInstructions)
	})

	t.Run("merges multiple tool instructions", func(t *testing.T) {
		var capturedInstructions string

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				capturedInstructions = req.Instructions
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("response"),
						Partial: false,
					}, nil)
				}
			},
		}

		tool1 := &mockToolWithInstruction{
			name:        "tool1",
			instruction: "Instruction for tool 1.",
		}
		tool2 := &mockToolWithInstruction{
			name:        "tool2",
			instruction: "Instruction for tool 2.",
		}

		nodeFn, err := NewModelNodeFunc(mdl,
			WithModelInstructions("Base prompt."),
			WithModelTools(tool1, tool2),
		)
		require.NoError(t, err)

		scope := createTestScope(map[string]any{
			graph.MessagesKeyName: []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		_, err = nodeFn(context.Background(), scope)
		require.NoError(t, err)

		// Verify instructions contains base prompt and both tool instructions
		assert.Contains(t, capturedInstructions, "Base prompt.")
		assert.Contains(t, capturedInstructions, "Instruction for tool 1.")
		assert.Contains(t, capturedInstructions, "Instruction for tool 2.")
	})
}
