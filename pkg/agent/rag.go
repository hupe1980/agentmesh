package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
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
func createRetrieveNode(retriever retrieval.Retriever) func(context.Context, state.ReadView) (*graph.Command, error) {
	return func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
		messages := GetMessages(view)
		if len(messages) == 0 {
			return nil, fmt.Errorf("no query messages")
		}

		query, err := extractUserQuery(messages)
		if err != nil {
			return nil, err
		}

		docs, err := retriever.Retrieve(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("retrieval failed: %w", err)
		}

		builder := graph.NewUpdate()
		graph.UpdateSet(builder, DocumentsKey, extractDocumentContent(docs))

		updates, err := builder.Build()
		if err != nil {
			return nil, err
		}

		return graph.Goto("generate", updates), nil
	}
}

// createGenerateNode creates the generation node for producing responses with context.
func createGenerateNode(mdl model.Model, config ragOptions) func(context.Context, state.ReadView) (*graph.Command, error) {
	return func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
		messages := GetMessages(view)

		docs := state.GetFromView(view, DocumentsKey)
		var updates state.Updates
		var err error

		if len(docs) == 0 {
			// No documents found, generate without context
			updates, err = generateWithModel(ctx, mdl, messages, "")
		} else {
			// Format context from documents
			contextPrompt := config.promptTemplate.MustRender(map[string]any{
				"Documents": docs,
			})
			updates, err = generateWithModel(ctx, mdl, messages, contextPrompt)
		}

		if err != nil {
			return nil, err
		}

		return graph.End(updates), nil
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

	mgr := state.NewManager()
	if err := RegisterMessagesKey(mgr); err != nil {
		return nil, fmt.Errorf("failed to register messages key: %w", err)
	}
	if err := state.RegisterKey(mgr, DocumentsKey); err != nil {
		return nil, fmt.Errorf("failed to register documents key: %w", err)
	}

	g, err := graph.NewGraph(mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to create graph: %w", err)
	}

	if err := g.AddNode(&graph.BaseCommandNode{
		NodeName:        "retrieve",
		Fn:              createRetrieveNode(retriever),
		DeclaredTargets: graph.NewTargetSet("generate"),
	}); err != nil {
		return nil, fmt.Errorf("failed to add retrieve node: %w", err)
	}

	if err := g.AddNode(&graph.BaseCommandNode{
		NodeName:        "generate",
		Fn:              createGenerateNode(mdl, config),
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
	}); err != nil {
		return nil, fmt.Errorf("failed to add generate node: %w", err)
	}

	// Entry point - Command pattern handles routing from retrieve → generate → END
	if err := g.SetEntryPoint("retrieve"); err != nil {
		return nil, fmt.Errorf("rag agent: failed to set entry point: %w", err)
	}

	// Compile the graph
	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		return nil, err
	}

	// Wrap with automatic callback injection if plugin manager is provided
	return WrapWithCallbacks(compiled, config.pluginManager), nil
}

// ragOptions holds configuration for RAG agents.
type ragOptions struct {
	promptTemplate *prompt.Template
	pluginManager  *callbacks.PluginManager
}

func defaultRAGOptions() ragOptions {
	tmpl := prompt.New(`Use the following documents to answer the question:

{{range .Documents}}
- {{.}}
{{end}}`)

	return ragOptions{
		promptTemplate: tmpl,
		pluginManager:  nil,
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

// WithRAGPluginManager sets the plugin manager for automatic callback injection.
func WithRAGPluginManager(pm *callbacks.PluginManager) RAGOption {
	return func(c *ragOptions) {
		c.pluginManager = pm
	}
}

// Helper function to generate response with optional context
func generateWithModel(ctx context.Context, mdl model.Model, msgs []message.Message, context string) (state.Updates, error) {
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

	// Return message in updates map
	builder := graph.NewUpdate()
	graph.UpdateAppend(builder, MessagesKey, resp.Message)

	return builder.Build()
}
