package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/command"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/prompt"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
	"github.com/hupe1980/agentmesh/pkg/state"
)

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
	return "", fmt.Errorf("no user query found")
}

// extractDocumentContent converts retrieval documents to string slices.
func extractDocumentContent(docs []retrieval.Document) []string {
	docStrings := make([]string, len(docs))
	for i, doc := range docs {
		docStrings[i] = doc.PageContent
	}
	return docStrings
}

// DocumentsKey is the state key for storing retrieved documents in RAG workflows.
var DocumentsKey = state.NewKey[[]string]("documents", nil)

// createRetrieveNode creates the retrieval node for fetching relevant documents.
func createRetrieveNode(retriever retrieval.Retriever) graph.NodeFunc {
	return func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
		messages := GetMessages(view)
		if len(messages) == 0 {
			return nil, nil, fmt.Errorf("no query messages")
		}

		query, err := extractUserQuery(messages)
		if err != nil {
			return nil, nil, err
		}

		docs, err := retriever.Retrieve(ctx, query)
		if err != nil {
			return nil, nil, fmt.Errorf("retrieval failed: %w", err)
		}

		return command.New().With(command.SetValue(DocumentsKey, extractDocumentContent(docs))).To("generate")
	}
}

// createGenerateNode creates the generation node for producing responses with context.
func createGenerateNode(mdl model.Model, config ragOptions) graph.NodeFunc {
	return func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
		messages := GetMessages(view)

		docs := state.GetFromView(view, DocumentsKey)
		var err error

		var newMsg message.Message
		if len(docs) == 0 {
			// No documents found, generate without context
			newMsg, err = generateWithModel(ctx, mdl, messages, "")
		} else {
			// Format context from documents
			contextPrompt := config.promptTemplate.MustRender(map[string]any{
				"Documents": docs,
			})
			newMsg, err = generateWithModel(ctx, mdl, messages, contextPrompt)
		}

		if err != nil {
			return nil, nil, err
		}

		return command.New().With(command.Append(MessagesKey, newMsg)).To(graph.EndNode)
	}
}

// NewRAGAgent creates a Retrieval-Augmented Generation agent that:
//  1. Retrieves relevant context from a knowledge base
//  2. Generates a response using both the query and retrieved context
//
// Returns a graph.Runnable for type-safe composition.
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
func NewRAGAgent(mdl model.Model, retriever retrieval.Retriever, opts ...RAGOption) (MessageRunnable, error) {
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

	mgr := state.NewManager()
	if err := RegisterMessagesKey(mgr); err != nil {
		return nil, fmt.Errorf("failed to register messages key: %w", err)
	}
	if err := state.RegisterKey(mgr, DocumentsKey); err != nil {
		return nil, fmt.Errorf("failed to register documents key: %w", err)
	}

	// Build graph using fluent builder API
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor(), graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		return nil, fmt.Errorf("failed to create builder: %w", err)
	}

	compiled, err := builder.
		AddNodeFunc("retrieve", []string{"generate"}, createRetrieveNode(retriever)).
		AddNodeFunc("generate", []string{graph.EndNode}, createGenerateNode(mdl, config)).
		SetEntryPoint("retrieve").
		Compile()
	if err != nil {
		return nil, fmt.Errorf("failed to build graph: %w", err)
	}

	return compiled, nil
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
