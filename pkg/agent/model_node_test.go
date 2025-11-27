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

func TestModelNode_WithOutputSchema(t *testing.T) {
	t.Run("sets output schema correctly", func(t *testing.T) {
		outputSchema := &schema.OutputSchema{
			Name:        "TestSchema",
			Description: "Test schema description",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{"type": "string"},
				},
			},
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("structured response"), nil
			}),
		}

		executor := model.NewExecutor(mdl)
		node, err := NewModelNode(executor, WithOutputSchema(outputSchema))
		require.NoError(t, err)
		require.NotNil(t, node)
		assert.Equal(t, outputSchema, node.outputSchema)
	})

	t.Run("handles nil output schema", func(t *testing.T) {
		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		// WithOutputSchema with nil should not panic
		node, err := NewModelNode(executor, WithOutputSchema(nil))
		require.NoError(t, err)
		require.NotNil(t, node)
		assert.Nil(t, node.outputSchema)
	})
}

func TestModelNode_Targets(t *testing.T) {
	t.Run("uses custom targets", func(t *testing.T) {
		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		customTargets := []string{"custom_node_1", "custom_node_2"}
		node, err := NewModelNode(executor, WithModelTargets(customTargets))
		require.NoError(t, err)

		// Verify targets are set correctly
		assert.Equal(t, customTargets, node.Targets())
	})

	t.Run("uses default targets", func(t *testing.T) {
		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		node, err := NewModelNode(executor)
		require.NoError(t, err)

		// Default targets should be "tool" and END
		assert.Equal(t, []string{"tool", graph.EndNode}, node.Targets())
	})
}

func TestModelNode_Configuration(t *testing.T) {
	t.Run("sets custom node name", func(t *testing.T) {
		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		node, err := NewModelNode(executor, WithModelNodeName("custom_model"))
		require.NoError(t, err)
		assert.Equal(t, "custom_model", node.Name())
	})

	t.Run("sets system prompt", func(t *testing.T) {
		customPrompt := "You are a helpful assistant"
		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		node, err := NewModelNode(executor, WithModelSystemPrompt(customPrompt))
		require.NoError(t, err)
		assert.Equal(t, customPrompt, node.systemPrompt)
	})

	t.Run("sets tools", func(t *testing.T) {
		tool1 := &testutil.MockTool{NameValue: "tool1"}
		tool2 := &testutil.MockTool{NameValue: "tool2"}

		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		node, err := NewModelNode(executor, WithModelTools(tool1, tool2))
		require.NoError(t, err)
		assert.Len(t, node.tools, 2)
		assert.Equal(t, "tool1", node.tools[0].Name())
		assert.Equal(t, "tool2", node.tools[1].Name())
	})

	t.Run("uses default values", func(t *testing.T) {
		mdl := &testutil.MockModel{}
		executor := model.NewExecutor(mdl)

		node, err := NewModelNode(executor)
		require.NoError(t, err)
		assert.Equal(t, "model", node.Name())
		assert.Equal(t, "", node.systemPrompt)
		assert.Empty(t, node.tools)
		assert.Equal(t, []string{"tool", graph.EndNode}, node.targets)
	})
}
