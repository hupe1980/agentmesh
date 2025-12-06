package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/prompt"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock Retriever
type mockRetriever struct {
	docs []retrieval.Document
	err  error
}

func (m *mockRetriever) Retrieve(ctx context.Context, query string) ([]retrieval.Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.docs, nil
}

// Tests for helper functions

func TestExtractUserQuery(t *testing.T) {
	t.Run("extracts last human message", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText("First question"),
			message.NewAIMessageFromText("First answer"),
			message.NewHumanMessageFromText("Second question"),
		}

		query, err := extractUserQuery(messages)
		require.NoError(t, err)
		assert.Equal(t, "Second question", query)
	})

	t.Run("finds human message among mixed types", func(t *testing.T) {
		messages := []message.Message{
			message.NewSystemMessageFromText("System prompt"),
			message.NewHumanMessageFromText("User query"),
			message.NewAIMessageFromText("AI response"),
		}

		query, err := extractUserQuery(messages)
		require.NoError(t, err)
		assert.Equal(t, "User query", query)
	})

	t.Run("returns error when no human message", func(t *testing.T) {
		messages := []message.Message{
			message.NewSystemMessageFromText("System prompt"),
			message.NewAIMessageFromText("AI response"),
		}

		_, err := extractUserQuery(messages)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user query found")
	})

	t.Run("returns error for empty message list", func(t *testing.T) {
		messages := []message.Message{}

		_, err := extractUserQuery(messages)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user query found")
	})

	t.Run("extracts from most recent human message", func(t *testing.T) {
		messages := []message.Message{
			message.NewHumanMessageFromText("Old question"),
			message.NewAIMessageFromText("Answer"),
			message.NewHumanMessageFromText("New question"),
			message.NewAIMessageFromText("Partial answer"),
		}

		query, err := extractUserQuery(messages)
		require.NoError(t, err)
		assert.Equal(t, "New question", query)
	})
}

func TestExtractDocumentContent(t *testing.T) {
	t.Run("extracts content from documents", func(t *testing.T) {
		docs := []retrieval.Document{
			{PageContent: "First doc content", Metadata: map[string]any{"source": "doc1"}},
			{PageContent: "Second doc content", Metadata: map[string]any{"source": "doc2"}},
			{PageContent: "Third doc content", Metadata: map[string]any{"source": "doc3"}},
		}

		content := extractDocumentContent(docs)
		require.Len(t, content, 3)
		assert.Equal(t, "First doc content", content[0])
		assert.Equal(t, "Second doc content", content[1])
		assert.Equal(t, "Third doc content", content[2])
	})

	t.Run("handles empty document list", func(t *testing.T) {
		docs := []retrieval.Document{}

		content := extractDocumentContent(docs)
		require.Len(t, content, 0)
	})

	t.Run("preserves document order", func(t *testing.T) {
		docs := []retrieval.Document{
			{PageContent: "Doc A"},
			{PageContent: "Doc B"},
			{PageContent: "Doc C"},
		}

		content := extractDocumentContent(docs)
		assert.Equal(t, []string{"Doc A", "Doc B", "Doc C"}, content)
	})
}

func TestGenerateWithModel(t *testing.T) {
	t.Run("generates without context", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				// Verify no extra system message
				assert.Len(t, messages, 1)
				assert.Equal(t, "What's the weather?", message.Stringify(messages[0]))
				return message.NewAIMessageFromText("Sunny"), nil
			}),
		}

		existingMsgs := []message.Message{
			message.NewHumanMessageFromText("What's the weather?"),
		}

		msg, err := generateWithModel(context.Background(), mdl, existingMsgs, "")

		require.NoError(t, err)
		assert.Equal(t, "Sunny", message.Stringify(msg))
	})

	t.Run("generates with context prepended", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				// Verify context is prepended as system message
				require.Len(t, messages, 2)
				assert.Equal(t, message.TypeSystem, messages[0].Type())
				assert.Equal(t, "Context: Weather is sunny", message.Stringify(messages[0]))
				assert.Equal(t, "What's the weather?", message.Stringify(messages[1]))
				return message.NewAIMessageFromText("Based on context: Sunny"), nil
			}),
		}

		existingMsgs := []message.Message{
			message.NewHumanMessageFromText("What's the weather?"),
		}

		msg, err := generateWithModel(context.Background(), mdl, existingMsgs, "Context: Weather is sunny")

		require.NoError(t, err)
		assert.Equal(t, "Based on context: Sunny", message.Stringify(msg))
	})

	t.Run("handles model error", func(t *testing.T) {
		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return nil, errors.New("model failed")
			}),
		}

		existingMsgs := []message.Message{
			message.NewHumanMessageFromText("Question"),
		}

		_, err := generateWithModel(context.Background(), mdl, existingMsgs, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "model failed")
	})
}

// Tests for RAG agent construction

func TestNewRAG_NilModel(t *testing.T) {
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "doc"}},
	}

	_, err := NewRAG(nil, retriever)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model")
}

func TestNewRAG_NilRetriever(t *testing.T) {
	mdl := &testutil.MockModel{}

	_, err := NewRAG(mdl, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retriever")
}

func TestNewRAG_ValidConstruction(t *testing.T) {
	mdl := &testutil.MockModel{}
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "test doc"}},
	}

	agent, err := NewRAG(mdl, retriever)

	require.NoError(t, err)
	require.NotNil(t, agent)
	// agent is *graph.MessageGraph
	_ = agent
}

func TestNewRAG_WithCustomPromptTemplate(t *testing.T) {
	mdl := &testutil.MockModel{}
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "test doc"}},
	}

	customTemplate := prompt.New("Custom: {{range .Documents}}{{.}}{{end}}")

	agent, err := NewRAG(mdl, retriever, WithPromptTemplate(customTemplate))

	require.NoError(t, err)
	require.NotNil(t, agent)
}

func TestNewRAG_WithNilPromptTemplate(t *testing.T) {
	mdl := &testutil.MockModel{}
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "test doc"}},
	}

	// Should use default template when nil passed
	agent, err := NewRAG(mdl, retriever, WithPromptTemplate(nil))

	require.NoError(t, err)
	require.NotNil(t, agent)
}

// End-to-end RAG workflow tests

func TestRAGAgent_RetrieveAndGenerate(t *testing.T) {
	t.Run("successful retrieval and generation", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{
				{PageContent: "The capital of France is Paris."},
				{PageContent: "Paris is known for the Eiffel Tower."},
			},
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				// Verify context was added
				hasContext := false
				for _, msg := range messages {
					if msg.Type() == message.TypeSystem {
						content := message.Stringify(msg)
						if len(content) > 0 && (contains(content, "France") || contains(content, "Paris")) {
							hasContext = true
						}
					}
				}
				assert.True(t, hasContext, "Expected context to be included in messages")
				return message.NewAIMessageFromText("The capital of France is Paris"), nil
			}),
		}

		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		input := []message.Message{
			message.NewHumanMessageFromText("What is the capital of France?"),
		}

		result, err := graph.Last(agent.Run(ctx, input))

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "The capital of France is Paris", message.Stringify(result))
	})

	t.Run("handles retrieval error", func(t *testing.T) {
		retriever := &mockRetriever{
			err: errors.New("retrieval failed"),
		}

		mdl := &testutil.MockModel{}

		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		input := []message.Message{
			message.NewHumanMessageFromText("Question"),
		}

		_, err = graph.Last(agent.Run(ctx, input))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retrieval failed")
	})

	t.Run("handles empty retrieval results", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{}, // No documents found
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				// Should generate without context when no docs found
				return message.NewAIMessageFromText("No documents available"), nil
			}),
		}

		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		input := []message.Message{
			message.NewHumanMessageFromText("Question about unknown topic"),
		}

		result, err := graph.Last(agent.Run(ctx, input))

		require.NoError(t, err)
		assert.Equal(t, "No documents available", message.Stringify(result))
	})

	t.Run("handles generation error", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{{PageContent: "test doc"}},
		}

		mdl := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
				return nil, errors.New("generation failed")
			}),
		}

		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		input := []message.Message{
			message.NewHumanMessageFromText("Question"),
		}

		_, err = graph.Last(agent.Run(ctx, input))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "generation failed")
	})
}

func TestRAGAgent_CustomPromptTemplate(t *testing.T) {
	retriever := &mockRetriever{
		docs: []retrieval.Document{
			{PageContent: "Doc 1 content"},
			{PageContent: "Doc 2 content"},
		},
	}

	customTemplate := prompt.New("CUSTOM CONTEXT:\n{{range .Documents}}\n> {{.}}{{end}}")

	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			// Verify custom template was used
			hasCustomFormat := false
			for _, msg := range messages {
				if msg.Type() == message.TypeSystem {
					content := message.Stringify(msg)
					if contains(content, "CUSTOM CONTEXT:") {
						hasCustomFormat = true
					}
				}
			}
			assert.True(t, hasCustomFormat, "Expected custom template format")
			return message.NewAIMessageFromText("Response using custom context"), nil
		}),
	}

	agent, err := NewRAG(mdl, retriever, WithPromptTemplate(customTemplate))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Question"),
	}

	result, err := graph.Last(agent.Run(ctx, input))

	require.NoError(t, err)
	assert.Equal(t, "Response using custom context", message.Stringify(result))
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsLoop(s, substr))
}

func containsLoop(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
