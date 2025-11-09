package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/prompt"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
)

// NewRAGAgent creates a Retrieval-Augmented Generation agent that:
//  1. Retrieves relevant context from a knowledge base
//  2. Generates a response using both the query and retrieved context
//
// This pattern is ideal for question-answering over large document collections.
//
// Example:
//
//	// Create retriever with topK configured
//	retriever := langchaingo.NewRetrieverFromVectorStore(vectorStore, func(o *langchaingo.Options) {
//	    o.NumDocuments = 5
//	})
//	agent, err := agent.NewRAGAgent(model, retriever)
func NewRAGAgent(mdl model.Model, retriever retrieval.Retriever, opts ...RAGOption) (*graph.Compiled, error) {
	if mdl == nil {
		return nil, fmt.Errorf("model must not be nil")
	}
	if retriever == nil {
		return nil, fmt.Errorf("retriever must not be nil")
	}

	config := defaultRAGOptions()
	for _, opt := range opts {
		opt(&config)
	}

	// Build RAG graph
	builder := graph.NewBuilder()

	// Retrieve node: fetch relevant documents
	builder.Node("retrieve", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		msgs := s.MessagesSnapshot()
		if len(msgs) == 0 {
			return nil, fmt.Errorf("no query messages")
		}

		// Get last user message as query
		var query string
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Type() == message.TypeHuman {
				// Get text from Parts
				for _, part := range msgs[i].Parts() {
					if textPart, ok := part.(message.TextPart); ok {
						query = textPart.Text
						break
					}
				}
				if query != "" {
					break
				}
			}
		}

		if query == "" {
			return nil, fmt.Errorf("no user query found")
		}

		// Retrieve documents
		docs, err := retriever.Retrieve(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("retrieval failed: %w", err)
		}

		// Extract page content from documents
		docStrings := make([]string, len(docs))
		for i, doc := range docs {
			docStrings[i] = doc.PageContent
		}

		// Store documents in state
		return &graph.NodeResult{
			Updates: map[string]any{
				"documents": docStrings,
			},
		}, nil
	})

	// Generate node: create response with context
	builder.Node("generate", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		docs, ok := s.Get("documents").([]string)
		if !ok || len(docs) == 0 {
			// No documents found, generate without context
			return generateWithModel(ctx, mdl, s.MessagesSnapshot(), "")
		}

		// Format context from documents
		contextPrompt := config.promptTemplate.MustRender(map[string]any{
			"Documents": docs,
		})

		return generateWithModel(ctx, mdl, s.MessagesSnapshot(), contextPrompt)
	})

	// Chain: retrieve → generate
	builder.AddEdge(graph.StartNode, "retrieve")
	builder.AddEdge("retrieve", "generate")
	builder.AddEdge("generate", graph.EndNode)

	return builder.Compile()
}

// MustNewRAGAgent is like NewRAGAgent but panics on error.
// Use this in tests or when you're certain inputs are valid.
func MustNewRAGAgent(mdl model.Model, retriever retrieval.Retriever, opts ...RAGOption) *graph.Compiled {
	agent, err := NewRAGAgent(mdl, retriever, opts...)
	if err != nil {
		panic(fmt.Errorf("failed to create RAG agent: %w", err))
	}
	return agent
}

// ragOptions holds configuration for RAG agents.
type ragOptions struct {
	promptTemplate *prompt.Template
}

func defaultRAGOptions() ragOptions {
	tmpl := prompt.New(`Use the following documents to answer the question:

{{range .Documents}}
- {{.}}
{{end}}`)

	return ragOptions{
		promptTemplate: tmpl,
	}
}

// RAGOption configures a RAG agent.
type RAGOption func(*ragOptions)

// WithPromptTemplate sets a custom prompt template for context formatting.
func WithPromptTemplate(tmpl *prompt.Template) RAGOption {
	return func(c *ragOptions) {
		if tmpl != nil {
			c.promptTemplate = tmpl
		}
	}
}

// Helper function to generate response with optional context
func generateWithModel(ctx context.Context, mdl model.Model, msgs []message.Message, context string) (*graph.NodeResult, error) {
	// Prepend context if provided
	if context != "" {
		contextMsg := message.NewSystemMessageFromText(context)
		msgs = append([]message.Message{contextMsg}, msgs...)
	}

	req := &model.Request{
		Messages: msgs,
	}

	resp, err := model.Last(mdl.Generate(ctx, req))
	if err != nil {
		return nil, err
	}

	return &graph.NodeResult{
		Messages: []message.Message{resp.Message},
	}, nil
}
