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
	return "", fmt.Errorf("agent/rag: no user query found")
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
			return graph.Fail(fmt.Errorf("agent/rag: no query messages"))
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
func createGenerateNode(mdl model.Model, config ragOptions) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(fmt.Errorf("agent/rag: no messages in state"))
		}

		docs := graph.Get(view, DocumentsKey)

		var newMsg message.Message
		var err error
		if len(docs) == 0 {
			// No documents found, generate without context
			newMsg, err = generateWithModel(ctx, mdl, msgs, "")
		} else {
			// Format context from documents
			contextPrompt := config.promptTemplate.MustRender(map[string]any{
				"Documents": docs,
			})
			newMsg, err = generateWithModel(ctx, mdl, msgs, contextPrompt)
		}

		if err != nil {
			return graph.Fail(err)
		}

		return graph.Append(MessagesKey, newMsg).To(graph.END)
	}
}

// NewRAGAgent creates a Retrieval-Augmented Generation agent that:
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
//	agent, err := agent.NewRAGAgent(model, retriever)
func NewRAGAgent(mdl model.Model, retriever retrieval.Retriever, opts ...RAGOption) (*message.CompiledMessageGraph, error) {
	if err := validate.All(
		validate.NotNil(mdl, "model"),
		validate.NotNil(retriever, "retriever"),
	); err != nil {
		return nil, err
	}

	config := defaultRAGOptions()
	for _, opt := range opts {
		opt(&config)
	}

	// Build graph - MessagesKey is automatically included by message.NewGraph
	g := message.NewGraph(DocumentsKey)
	g.Node("retrieve", createRetrieveNode(retriever), "generate")
	g.Node("generate", createGenerateNode(mdl, config), graph.END)
	g.Start("retrieve")

	return g.Build()
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

// generateWithModel generates a response with optional context.
func generateWithModel(ctx context.Context, mdl model.Model, existingMsgs []message.Message, context string) (message.Message, error) {
	// Build request messages with optional context prepended
	requestMsgs := existingMsgs
	if context != "" {
		contextMsg := message.NewSystemMessageFromText(context)
		requestMsgs = append([]message.Message{contextMsg}, existingMsgs...)
	}

	req := &model.Request{
		Messages: requestMsgs,
	}

	resp, err := model.Last(mdl.Generate(ctx, req))
	if err != nil {
		return nil, err
	}

	// Return only the NEW message
	return resp.Message, nil
}
