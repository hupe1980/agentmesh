package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/prompt"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
)

// DocumentsKey is the state key for storing retrieved documents in RAG workflows.
var DocumentsKey = graph.NewKey[[]string]("documents")

// RephrasedQueryKey stores the rephrased query for retrieval.
// This is set by the rephrase node when query rephrasing is enabled.
var RephrasedQueryKey = graph.NewKey[string]("rephrased_query")

// extractUserQuery finds the last human message text from messages.
func extractUserQuery(messages []message.Message) (string, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Type() == message.TypeHuman {
			return messages[i].String(), nil
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

// defaultRephrasePrompt is the default prompt template for query rephrasing.
var defaultRephrasePrompt = prompt.New(`Given the conversation history and a follow-up question, rephrase the follow-up question to be a standalone question that contains all necessary context.

Conversation history:
{{range .History}}
{{.Type}}: {{.String}}
{{end}}

Follow-up question: {{.Query}}

Standalone question:`)

// createRephraseNode creates a node that rephrases queries in conversational contexts.
// It automatically detects if conversation history exists and skips rephrasing for standalone queries.
func createRephraseNode(executor model.Executor, rephrasePrompt *prompt.Template) message.NodeFunc {
	return func(ctx context.Context, scope message.Scope) (*graph.Command, error) {
		logger := logging.FromContext(ctx)

		msgs := GetMessages(scope)
		if len(msgs) == 0 {
			return graph.Fail(ErrNoQueryMessages)
		}

		// Extract current query
		query, err := extractUserQuery(msgs)
		if err != nil {
			return graph.Fail(err)
		}

		// AUTO-DETECTION: Check if we're in a conversational context
		if !IsConversationalContext(scope) {
			// No conversation history - skip rephrasing, use query as-is
			logger.Debug("skipping query rephrasing - no conversation history")
			return graph.Set(RephrasedQueryKey, query).To("retrieve")
		}

		// Get conversation history for context
		history := GetConversationHistory(msgs)
		if len(history) == 0 {
			// No usable history - skip rephrasing
			logger.Debug("skipping query rephrasing - no usable history")
			return graph.Set(RephrasedQueryKey, query).To("retrieve")
		}

		// Build rephrase prompt
		promptText := rephrasePrompt.MustRender(map[string]any{
			"History": history,
			"Query":   query,
		})

		// Call model to rephrase
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText(promptText),
			},
		}

		resp, err := graph.Last(executor.Generate(ctx, req))
		if err != nil {
			// Fall back to original query on error (don't break the pipeline)
			logger.Warn("query rephrasing failed, using original query",
				"error", err,
				"query", query,
			)
			return graph.Set(RephrasedQueryKey, query).To("retrieve")
		}

		rephrased := strings.TrimSpace(resp.Message.String())
		logger.Debug("query rephrased",
			"original", query,
			"rephrased", rephrased,
		)

		return graph.Set(RephrasedQueryKey, rephrased).To("retrieve")
	}
}

// createRetrieveNode creates the retrieval node for fetching relevant documents.
func createRetrieveNode(retriever retrieval.Retriever) message.NodeFunc {
	return func(ctx context.Context, scope message.Scope) (*graph.Command, error) {
		// Check for rephrased query first (set by rephrase node)
		query := graph.Get(scope, RephrasedQueryKey)

		// Fall back to extracting from messages if no rephrased query
		if query == "" {
			msgs := GetMessages(scope)
			if len(msgs) == 0 {
				return graph.Fail(ErrNoQueryMessages)
			}

			var err error
			query, err = extractUserQuery(msgs)
			if err != nil {
				return graph.Fail(err)
			}
		}

		docs, err := retriever.Retrieve(ctx, query)
		if err != nil {
			return graph.Fail(fmt.Errorf("agent/rag: retrieval failed: %w", err))
		}

		return graph.Set(DocumentsKey, extractDocumentContent(docs)).To("generate")
	}
}

// createRAGInstructionsFunc creates a dynamic instructions function that combines
// user instructions with document context retrieved from state.
func createRAGInstructionsFunc(config ragOptions) func(context.Context, message.Scope) (string, error) {
	return func(ctx context.Context, scope message.Scope) (string, error) {
		var instructions string

		// First resolve base instructions if configured
		if config.instructions != nil {
			var err error
			instructions, err = config.instructions.Resolve(ctx, scope)
			if err != nil {
				return "", fmt.Errorf("failed to resolve instructions: %w", err)
			}
		}

		// Add document context if documents exist
		docs := graph.Get(scope, DocumentsKey)
		if len(docs) > 0 {
			contextText := config.contextPrompt.MustRender(map[string]any{
				"Documents": docs,
			})
			if instructions != "" {
				instructions = instructions + "\n\n" + contextText
			} else {
				instructions = contextText
			}
		}

		return instructions, nil
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

	// Build model node options
	modelNodeOpts := []ModelNodeOption{
		WithModelInstructionsFunc(createRAGInstructionsFunc(config)),
		WithModelStreaming(config.streaming),
	}

	// Add output schema if configured
	if config.outputSchema != nil {
		modelNodeOpts = append(modelNodeOpts, WithModelOutputSchema(config.outputSchema))
	}

	// Create model node function using the shared ModelNodeFunc
	modelFn, err := NewModelNodeFunc(modelExecutor, modelNodeOpts...)
	if err != nil {
		return nil, fmt.Errorf("agent/rag: create model node: %w", err)
	}

	// Build graph - MessagesKey is automatically included by message.NewGraphBuilder
	b := message.NewGraphBuilder(DocumentsKey, RephrasedQueryKey)

	// Include rephrase node by default - it auto-skips when no conversation history exists
	// Graph structure with rephrasing: START → rephrase → retrieve → generate → END
	// Graph structure without rephrasing: START → retrieve → generate → END
	if config.skipRephrasing {
		b.Node("retrieve", createRetrieveNode(retriever), "generate")
		b.Node("generate", modelFn, graph.END)
		b.Start("retrieve")
	} else {
		b.Node("rephrase", createRephraseNode(modelExecutor, config.rephrasePrompt), "retrieve")
		b.Node("retrieve", createRetrieveNode(retriever), "generate")
		b.Node("generate", modelFn, graph.END)
		b.Start("rephrase")
	}

	// Apply node middleware if provided
	if len(config.nodeMiddleware) > 0 {
		b.WithNodeMiddleware(config.nodeMiddleware...)
	}

	// Apply run middleware if provided (wraps Run/Resume)
	if len(config.runMiddleware) > 0 {
		b.WithRunMiddleware(config.runMiddleware...)
	}

	return b.Build()
}

// ragOptions holds configuration for RAG agents.
type ragOptions struct {
	commonOptions
	contextPrompt  *prompt.Template // Prompt for formatting retrieved documents
	rephrasePrompt *prompt.Template // Prompt for rephrasing queries in conversations
	skipRephrasing bool             // Disable automatic query rephrasing
}

func defaultRAGOptions() ragOptions {
	contextTmpl := prompt.New(`Use the following documents to answer the question:

{{range .Documents}}
- {{.}}
{{end}}`)

	return ragOptions{
		commonOptions: commonOptions{
			instructions:    nil,
			maxIterations:   1, // RAG is typically single-pass
			outputSchema:    nil,
			nodeMiddleware:  nil,
			runMiddleware:   nil,
			modelMiddleware: nil,
			toolMiddleware:  nil,
		},
		contextPrompt:  contextTmpl,
		rephrasePrompt: defaultRephrasePrompt,
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

// WithContextPrompt sets a custom prompt template for formatting retrieved documents.
// This prompt is used to present the retrieved context to the model for generation.
func WithContextPrompt(tmpl *prompt.Template) RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		if tmpl != nil {
			c.contextPrompt = tmpl
		}
	})
}

// WithSkipRephrasing disables automatic query rephrasing.
//
// By default, the RAG agent automatically detects conversational context and
// rephrases follow-up questions to be standalone queries for better retrieval.
// Use this option to disable this behavior if you want to use queries as-is.
//
// Example:
//
//	// Disable automatic rephrasing
//	ragAgent, _ := agent.NewRAG(model, retriever,
//	    agent.WithSkipRephrasing(),
//	)
func WithSkipRephrasing() RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		c.skipRephrasing = true
	})
}

// WithRephrasePrompt sets a custom prompt template for query rephrasing.
// This is used when the RAG agent rephrases queries in conversational contexts.
// Has no effect if WithSkipRephrasing is also used.
func WithRephrasePrompt(tmpl *prompt.Template) RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		if tmpl != nil {
			c.rephrasePrompt = tmpl
		}
	})
}
