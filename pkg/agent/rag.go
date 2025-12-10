package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/prompt"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
)

// DocumentsKey is the state key for storing retrieved documents in RAG workflows.
var DocumentsKey = graph.NewKey[[]string]("documents", nil)

// extractUserQuery finds the last human message text from messages.
func extractUserQuery(messages []message.Message) (string, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Type() == message.TypeHuman {
			// Get text from Parts
			for _, part := range messages[i].Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					return textPart.Text, nil
				}
			}
		}
	}
	return "", ErrNoUserQuery
}

// extractDocumentContent converts retrieval documents to string slices.
func extractDocumentContent(docs []retrieval.Document) []string {
	docStrings := make([]string, len(docs))
	for i, doc := range docs {
		docStrings[i] = doc.PageContent
	}
	return docStrings
}

// createRetrieveNode creates the retrieval node for fetching relevant documents.
func createRetrieveNode(retriever retrieval.Retriever) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(ErrNoQueryMessages)
		}

		query, err := extractUserQuery(msgs)
		if err != nil {
			return graph.Fail(err)
		}

		docs, err := retriever.Retrieve(ctx, query)
		if err != nil {
			return graph.Fail(fmt.Errorf("agent/rag: retrieval failed: %w", err))
		}

		return graph.Set(DocumentsKey, extractDocumentContent(docs)).To("generate")
	}
}

// createGenerateNode creates the generation node for producing responses with context.
func createGenerateNode(executor model.Executor, config ragOptions) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(ErrNoMessagesInState)
		}

		docs := graph.Get(view, DocumentsKey)

		// Build instructions: combine user's instructions with document context
		var instructions string

		// First resolve base instructions if configured
		if config.instructions != nil {
			var err error
			instructions, err = config.instructions.Resolve(ctx, view)
			if err != nil {
				return graph.Fail(fmt.Errorf("failed to resolve instructions: %w", err))
			}
		}

		// Add document context if documents exist
		if len(docs) > 0 {
			contextPrompt := config.promptTemplate.MustRender(map[string]any{
				"Documents": docs,
			})
			if instructions != "" {
				instructions = instructions + "\n\n" + contextPrompt
			} else {
				instructions = contextPrompt
			}
		}

		req := &model.Request{
			Messages:     msgs,
			Instructions: instructions,
			OutputSchema: config.outputSchema,
		}

		resp, err := model.Last(executor.Generate(ctx, req))
		if err != nil {
			return graph.Fail(err)
		}

		return graph.Append(MessagesKey, resp.Message).To(graph.END)
	}
}

// NewRAG creates a Retrieval-Augmented Generation agent that:
//  1. Retrieves relevant context from a knowledge base
//  2. Generates a response using both the query and retrieved context
//
// Returns a *graph.MessageGraph for type-safe composition.
//
// This pattern is ideal for question-answering over large document collections.
//
// Example:
//
//	// Create retriever with topK configured
//	retriever := langchaingo.NewRetrieverFromVectorStore(vectorStore, func(o *langchaingo.Options) {
//	    o.NumDocuments = 5
//	})
//	agent, err := agent.NewRAG(model, retriever)
func NewRAG(mdl model.Model, retriever retrieval.Retriever, opts ...RAGOption) (*message.Graph, error) {
	if err := validate.All(
		validate.NotNil(mdl, "model"),
		validate.NotNil(retriever, "retriever"),
	); err != nil {
		return nil, err
	}

	config := defaultRAGOptions()
	for _, opt := range opts {
		opt.applyRAG(&config)
	}

	// Create model executor with middleware
	modelExecutor := model.NewExecutor(mdl, model.WithExecutorName("rag-model"))
	if len(config.modelMiddleware) > 0 {
		modelExecutor = model.Chain(modelExecutor, config.modelMiddleware...)
	}

	// Build graph - MessagesKey is automatically included by message.NewGraphBuilder
	b := message.NewGraphBuilder(DocumentsKey)
	b.Node("retrieve", createRetrieveNode(retriever), "generate")
	b.Node("generate", createGenerateNode(modelExecutor, config), graph.END)
	b.Start("retrieve")

	// Apply graph middleware if provided
	if len(config.graphMiddleware) > 0 {
		b.WithMiddleware(config.graphMiddleware...)
	}

	return b.Build()
}

// ragOptions holds configuration for RAG agents.
type ragOptions struct {
	commonOptions
	promptTemplate *prompt.Template
}

func defaultRAGOptions() ragOptions {
	tmpl := prompt.New(`Use the following documents to answer the question:

{{range .Documents}}
- {{.}}
{{end}}`)

	return ragOptions{
		commonOptions: commonOptions{
			instructions:    nil,
			maxIterations:   1, // RAG is typically single-pass
			outputSchema:    nil,
			graphMiddleware: nil,
			modelMiddleware: nil,
			toolMiddleware:  nil,
		},
		promptTemplate: tmpl,
	}
}

// RAGOption configures a RAG agent.
type RAGOption interface {
	applyRAG(*ragOptions)
}

// ragOptionFunc wraps a function to implement RAGOption.
type ragOptionFunc func(*ragOptions)

func (f ragOptionFunc) applyRAG(opts *ragOptions) {
	f(opts)
}

// WithPromptTemplate sets a custom prompt template for context formatting.
func WithPromptTemplate(tmpl *prompt.Template) RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		if tmpl != nil {
			c.promptTemplate = tmpl
		}
	})
}
