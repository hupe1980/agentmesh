package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
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

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor)
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("returns error for nil executor", func(t *testing.T) {
		_, err := NewModelNodeFunc(nil)
		require.Error(t, err)
	})

	t.Run("with system prompt", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("response"), nil
			}),
		}

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor, WithModelSystemPrompt("You are helpful"))
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

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor, WithModelOutputSchema(outputSchema))
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

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor, WithModelTools(tool1, tool2))
		require.NoError(t, err)
		require.NotNil(t, nodeFn)
	})

	t.Run("with custom tool target", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("response"), nil
			}),
		}

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor, WithToolTarget("custom_tool_node"))
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

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor)
		require.NoError(t, err)

		// Create a view with messages
		view := createTestView(map[string]any{
			MessagesKey.Name(): []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		cmd, err := nodeFn(context.Background(), view)
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

		executor := model.NewExecutor(mdl)
		nodeFn, err := NewModelNodeFunc(executor)
		require.NoError(t, err)

		view := createTestView(map[string]any{
			MessagesKey.Name(): []message.Message{
				message.NewHumanMessageFromText("Use a tool"),
			},
		})

		cmd, err := nodeFn(context.Background(), view)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		// Should route to tool node
		assert.Contains(t, cmd.Next, "tool")
	})
}

// createTestView creates a View for testing using BSPState
func createTestView(data map[string]any) graph.View {
	return graph.NewBSPState(data).ReadView()
}
