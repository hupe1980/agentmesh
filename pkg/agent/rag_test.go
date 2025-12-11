package agent

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/prompt"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
	"github.com/hupe1980/agentmesh/pkg/testutil"
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
	mdl := testutil.NewModelBuilder().Build()

	_, err := NewRAG(mdl, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retriever")
}

func TestNewRAG_ValidConstruction(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "test doc"}},
	}

	agent, err := NewRAG(mdl, retriever)

	require.NoError(t, err)
	require.NotNil(t, agent)
	// agent is *graph.MessageGraph
	_ = agent
}

func TestNewRAG_WithCustomContextPrompt(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "test doc"}},
	}

	customTemplate := prompt.New("Custom: {{range .Documents}}{{.}}{{end}}")

	agent, err := NewRAG(mdl, retriever, WithContextPrompt(customTemplate))

	require.NoError(t, err)
	require.NotNil(t, agent)
}

func TestNewRAG_WithNilContextPrompt(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()
	retriever := &mockRetriever{
		docs: []retrieval.Document{{PageContent: "test doc"}},
	}

	// Should use default template when nil passed
	agent, err := NewRAG(mdl, retriever, WithContextPrompt(nil))

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
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				// Verify context was added via Instructions
				hasContext := contains(req.Instructions, "France") || contains(req.Instructions, "Paris")
				assert.True(t, hasContext, "Expected context to be included in Instructions")
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("The capital of France is Paris"),
						Partial: false,
					}, nil)
				}
			},
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
		assert.Equal(t, "The capital of France is Paris", result.String())
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
		assert.Equal(t, "No documents available", result.String())
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

func TestRAGAgent_CustomContextPrompt(t *testing.T) {
	retriever := &mockRetriever{
		docs: []retrieval.Document{
			{PageContent: "Doc 1 content"},
			{PageContent: "Doc 2 content"},
		},
	}

	customTemplate := prompt.New("CUSTOM CONTEXT:\n{{range .Documents}}\n> {{.}}{{end}}")

	mdl := &testutil.MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			// Verify custom template was used in Instructions
			hasCustomFormat := contains(req.Instructions, "CUSTOM CONTEXT:")
			assert.True(t, hasCustomFormat, "Expected custom template format in Instructions")
			return func(yield func(*model.Response, error) bool) {
				yield(&model.Response{
					Message: message.NewAIMessageFromText("Response using custom context"),
					Partial: false,
				}, nil)
			}
		},
	}

	agent, err := NewRAG(mdl, retriever, WithContextPrompt(customTemplate))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Question"),
	}

	result, err := graph.Last(agent.Run(ctx, input))

	require.NoError(t, err)
	assert.Equal(t, "Response using custom context", result.String())
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

func TestRAGAgent_QueryRephrasing(t *testing.T) {
	t.Run("skips rephrasing for standalone query", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{
				{PageContent: "Acme pricing doc"},
			},
		}

		rephraseCalled := false
		generateCalled := false

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				// Check if this is a rephrase call (contains "Standalone question")
				if contains(req.Messages[0].String(), "Standalone question") {
					rephraseCalled = true
				} else {
					generateCalled = true
				}
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("Response"),
						Partial: false,
					}, nil)
				}
			},
		}

		// Rephrasing is enabled by default, but skips for standalone queries
		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		// Single message - no conversation history
		input := []message.Message{
			message.NewHumanMessageFromText("What is Acme Corp pricing?"),
		}

		_, err = graph.Last(agent.Run(ctx, input))
		require.NoError(t, err)

		// Rephrase should be skipped for standalone query
		assert.False(t, rephraseCalled, "Rephrase should be skipped for standalone query")
		assert.True(t, generateCalled, "Generate should be called")
	})

	t.Run("rephrases query in conversational context", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{
				{PageContent: "Acme pricing: $99/month"},
			},
		}

		rephraseCalled := false
		generateCalled := false

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				// Check if this is a rephrase call
				if contains(req.Messages[0].String(), "Standalone question") {
					rephraseCalled = true
					return func(yield func(*model.Response, error) bool) {
						// Return rephrased query
						yield(&model.Response{
							Message: message.NewAIMessageFromText("What is Acme Corp's pricing?"),
							Partial: false,
						}, nil)
					}
				}
				generateCalled = true
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("Acme pricing is $99/month"),
						Partial: false,
					}, nil)
				}
			},
		}

		// Rephrasing is enabled by default
		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		// Conversation history - should trigger rephrasing
		input := []message.Message{
			message.NewHumanMessageFromText("Tell me about Acme Corp"),
			message.NewAIMessageFromText("Acme Corp is a software company..."),
			message.NewHumanMessageFromText("What about their pricing?"),
		}

		result, err := graph.Last(agent.Run(ctx, input))
		require.NoError(t, err)

		assert.True(t, rephraseCalled, "Rephrase should be called for conversational context")
		assert.True(t, generateCalled, "Generate should be called")
		assert.Equal(t, "Acme pricing is $99/month", result.String())
	})

	t.Run("falls back to original query on rephrase error", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{
				{PageContent: "Some doc"},
			},
		}

		rephraseAttempted := false
		generateCalled := false

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				if contains(req.Messages[0].String(), "Standalone question") {
					rephraseAttempted = true
					// Return error for rephrase
					return func(yield func(*model.Response, error) bool) {
						yield(nil, errors.New("rephrase failed"))
					}
				}
				generateCalled = true
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("Response"),
						Partial: false,
					}, nil)
				}
			},
		}

		// Rephrasing is enabled by default
		agent, err := NewRAG(mdl, retriever)
		require.NoError(t, err)

		ctx := context.Background()
		input := []message.Message{
			message.NewHumanMessageFromText("First question"),
			message.NewAIMessageFromText("First answer"),
			message.NewHumanMessageFromText("Follow-up question"),
		}

		result, err := graph.Last(agent.Run(ctx, input))
		require.NoError(t, err)

		assert.True(t, rephraseAttempted, "Rephrase should be attempted")
		assert.True(t, generateCalled, "Generate should be called even after rephrase error")
		assert.Equal(t, "Response", result.String())
	})

	t.Run("skips rephrasing when WithSkipRephrasing is set", func(t *testing.T) {
		retriever := &mockRetriever{
			docs: []retrieval.Document{
				{PageContent: "Acme pricing: $99/month"},
			},
		}

		rephraseCalled := false
		generateCalled := false

		mdl := &testutil.MockModel{
			GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
				// Check if this is a rephrase call
				if contains(req.Messages[0].String(), "Standalone question") {
					rephraseCalled = true
				}
				generateCalled = true
				return func(yield func(*model.Response, error) bool) {
					yield(&model.Response{
						Message: message.NewAIMessageFromText("Response"),
						Partial: false,
					}, nil)
				}
			},
		}

		// Explicitly disable rephrasing
		agent, err := NewRAG(mdl, retriever, WithSkipRephrasing())
		require.NoError(t, err)

		ctx := context.Background()
		// Even with conversation history, rephrasing should be skipped
		input := []message.Message{
			message.NewHumanMessageFromText("Tell me about Acme Corp"),
			message.NewAIMessageFromText("Acme Corp is a software company..."),
			message.NewHumanMessageFromText("What about their pricing?"),
		}

		_, err = graph.Last(agent.Run(ctx, input))
		require.NoError(t, err)

		assert.False(t, rephraseCalled, "Rephrase should be skipped when WithSkipRephrasing is set")
		assert.True(t, generateCalled, "Generate should be called")
	})
}
