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

// GroundingMode defines how strictly the model should ground responses.
type GroundingMode int

const (
	// GroundingStrict requires all claims to be directly from documents.
	// Model will refuse to answer if information isn't in the context.
	// This is the default mode.
	GroundingStrict GroundingMode = iota

	// GroundingGuided prefers document information but allows inferences.
	// Model clearly distinguishes sourced facts from inferences.
	GroundingGuided

	// GroundingCitation requires inline citations for all claims.
	// Enables source attribution but allows some inference.
	GroundingCitation

	// GroundingNone disables grounding prompts entirely.
	// The model can use any knowledge to answer questions.
	GroundingNone
)

// CitationStyle defines how citations are formatted in responses.
type CitationStyle int

const (
	// CitationBracket uses bracket notation: [1], [2]
	CitationBracket CitationStyle = iota

	// CitationSuperscript uses superscript notation: ¹, ²
	CitationSuperscript

	// CitationParenthetical uses parenthetical notation: (1), (2)
	CitationParenthetical
)

// citationFormats maps citation styles to their format strings.
var citationFormats = map[CitationStyle]struct {
	format  string // Format for citations, e.g., "[%d]"
	example string // Example showing the style in use
}{
	CitationBracket:       {format: "[%d]", example: "The capital of France is Paris [1]. It has a population of over 2 million [2]."},
	CitationSuperscript:   {format: "⁽%d⁾", example: "The capital of France is Paris⁽¹⁾. It has a population of over 2 million⁽²⁾."},
	CitationParenthetical: {format: "(%d)", example: "The capital of France is Paris (1). It has a population of over 2 million (2)."},
}

// groundingPromptTemplates contains the default prompt templates for each grounding mode.
var groundingPromptTemplates = map[GroundingMode]*prompt.Template{
	GroundingStrict: prompt.New(`STRICT GROUNDING RULES:
1. ONLY use information explicitly stated in the documents above
2. If the answer is not in the documents, say: "Based on the provided documents, I cannot answer this question."
3. Do NOT use any prior knowledge or make assumptions
4. Do NOT infer or extrapolate beyond what is directly stated
5. If partially answerable, answer what you can and state what's missing`),

	GroundingGuided: prompt.New(`GROUNDING GUIDELINES:
1. Prefer information directly from the documents above
2. Clearly distinguish between:
   - Facts from documents: "According to the documents..."
   - Reasonable inferences: "Based on this, it seems..."
   - General knowledge: "Generally speaking..."
3. When documents are insufficient, acknowledge limitations
4. Always prioritize accuracy over completeness`),

	GroundingCitation: prompt.New(`CITATION RULES:
1. Include a citation {{.CitationFormat}} after each factual claim from the documents above
2. Citations reference the document number in order of appearance
3. If making an inference, prefix with "Based on {{.CitationFormat}}..."
4. At the end, optionally list sources if helpful
5. If information isn't in documents, say so without citation

Example: "{{.CitationExample}}"`),
}

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
func createRephraseNode(mdl model.Model, rephrasePrompt *prompt.Template) graph.NodeFunc {
	// Create a simple executor for rephrasing (no middleware needed)
	executor := model.NewExecutor(mdl, model.WithExecutorName("rag-rephrase"))

	return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		logger := logging.FromContext(ctx)

		msgs := scope.Messages()
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
func createRetrieveNode(retriever retrieval.Retriever) graph.NodeFunc {
	return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Check for rephrased query first (set by rephrase node)
		query := graph.Get(scope, RephrasedQueryKey)

		// Fall back to extracting from messages if no rephrased query
		if query == "" {
			msgs := scope.Messages()
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

		docStrings := extractDocumentContent(docs)

		return graph.Set(DocumentsKey, docStrings).To("generate")
	}
}

// createRAGInstructionsFunc creates a dynamic instructions function that combines
// user instructions, document context, and grounding prompts.
// Order: 1) User instructions, 2) Documents, 3) Grounding rules (last for emphasis)
func createRAGInstructionsFunc(config ragOptions) func(context.Context, graph.Scope) (string, error) {
	// Pre-render citation format for the grounding prompt
	citationInfo := citationFormats[config.citationStyle]

	return func(ctx context.Context, scope graph.Scope) (string, error) {
		var sb strings.Builder

		// 1. Add user instructions first (persona/task definition)
		if config.instructions != nil {
			instructions, err := config.instructions.Resolve(ctx, scope)
			if err != nil {
				return "", fmt.Errorf("failed to resolve instructions: %w", err)
			}
			sb.WriteString(instructions)
			sb.WriteString("\n\n")
		}

		// 2. Add document context (the source material)
		docs := graph.Get(scope, DocumentsKey)
		if len(docs) > 0 {
			// Use numbered format for citation mode, simple format otherwise
			if config.groundingMode == GroundingCitation {
				sb.WriteString("CONTEXT DOCUMENTS:\n")
				for i, doc := range docs {
					fmt.Fprintf(&sb, "\n[Document %d]\n%s\n", i+1, doc)
				}
			} else {
				contextText := config.contextPrompt.MustRender(map[string]any{
					"Documents": docs,
				})
				sb.WriteString(contextText)
			}
			sb.WriteString("\n")
		}

		// 3. Add grounding prompt last (rules enforcement - recency bias)
		//nolint:nestif // Conditional grounding logic requires nested structure
		if config.groundingMode != GroundingNone {
			var groundingText string
			var err error

			templateData := map[string]any{
				"CitationFormat":  fmt.Sprintf(citationInfo.format, 'n'),
				"CitationExample": citationInfo.example,
			}

			if config.groundingPrompt != "" {
				// Render custom grounding prompt as template
				customTmpl := prompt.New(config.groundingPrompt)
				groundingText, err = customTmpl.Render(templateData)
				if err != nil {
					return "", fmt.Errorf("failed to render custom grounding prompt: %w", err)
				}
			} else {
				// Render default grounding prompt template
				tmpl := groundingPromptTemplates[config.groundingMode]
				groundingText, err = tmpl.Render(templateData)
				if err != nil {
					return "", fmt.Errorf("failed to render grounding prompt: %w", err)
				}
			}
			sb.WriteString(groundingText)
		}

		return sb.String(), nil
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
func NewRAG(mdl model.Model, retriever retrieval.Retriever, opts ...RAGOption) (*graph.Graph, error) {
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

	// Build model node options
	modelNodeOpts := []ModelNodeOption{
		WithModelName("rag-model"),
		WithModelInstructionsFunc(createRAGInstructionsFunc(config)),
		WithModelStreaming(config.streaming),
	}

	// Add middleware if configured
	if len(config.modelMiddleware) > 0 {
		modelNodeOpts = append(modelNodeOpts, WithModelNodeMiddleware(config.modelMiddleware...))
	}

	// Add output schema if configured
	if config.outputSchema != nil {
		modelNodeOpts = append(modelNodeOpts, WithModelOutputSchema(config.outputSchema))
	}

	// Create model node function using the shared ModelNodeFunc
	modelFn, err := NewModelNodeFunc(mdl, modelNodeOpts...)
	if err != nil {
		return nil, fmt.Errorf("agent/rag: create model node: %w", err)
	}

	// Build graph - MessagesKey is automatically included by graph.New
	b := graph.New(DocumentsKey, RephrasedQueryKey)

	// Include rephrase node by default - it auto-skips when no conversation history exists
	// Graph structure with rephrasing: START → rephrase → retrieve → generate → END
	// Graph structure without rephrasing: START → retrieve → generate → END
	if config.skipRephrasing {
		b.Node("retrieve", createRetrieveNode(retriever), "generate")
		b.Node("generate", modelFn, graph.END)
		b.Start("retrieve")
	} else {
		b.Node("rephrase", createRephraseNode(mdl, config.rephrasePrompt), "retrieve")
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
	contextPrompt   *prompt.Template // Prompt for formatting retrieved documents
	rephrasePrompt  *prompt.Template // Prompt for rephrasing queries in conversations
	skipRephrasing  bool             // Disable automatic query rephrasing
	groundingMode   GroundingMode    // Grounding mode (default: GroundingStrict)
	groundingPrompt string           // Custom grounding prompt (overrides default)
	citationStyle   CitationStyle    // Citation format for GroundingCitation mode
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
		contextPrompt:   contextTmpl,
		rephrasePrompt:  defaultRephrasePrompt,
		groundingMode:   GroundingStrict, // Grounding enabled by default
		groundingPrompt: "",
		citationStyle:   CitationBracket, // Default citation style
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

// WithGroundingMode sets the grounding mode for RAG responses.
// By default, GroundingStrict is used which requires all claims to be from documents.
//
// Available modes:
//   - GroundingStrict: Only answer from provided documents (default)
//   - GroundingGuided: Prefer documents but allow general knowledge
//   - GroundingCitation: Require inline citations for all claims
//   - GroundingNone: Disable grounding prompts entirely
//
// Example:
//
//	ragAgent, _ := agent.NewRAG(model, retriever,
//	    agent.WithGroundingMode(agent.GroundingGuided),
//	)
func WithGroundingMode(mode GroundingMode) RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		c.groundingMode = mode
	})
}

// WithoutGrounding disables grounding prompts entirely.
// This is a shorthand for WithGroundingMode(GroundingNone).
//
// Use this when you want the model to use any knowledge to answer,
// not just the retrieved documents.
//
// Example:
//
//	ragAgent, _ := agent.NewRAG(model, retriever,
//	    agent.WithoutGrounding(),
//	)
func WithoutGrounding() RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		c.groundingMode = GroundingNone
	})
}

// WithGroundingPrompt sets a custom grounding prompt.
// This overrides the default prompt for the selected grounding mode.
//
// The prompt is rendered as a template with the following variables:
//   - {{.CitationFormat}}: The citation format string (e.g., "[n]", "(n)")
//   - {{.CitationExample}}: An example sentence with citations
//
// Note: When using a custom prompt, you have full control over grounding behavior.
// The groundingMode still affects document formatting (numbered for citation mode).
//
// Example:
//
//	ragAgent, _ := agent.NewRAG(model, retriever,
//	    agent.WithGroundingPrompt("Use citations {{.CitationFormat}} for all claims..."),
//	)
func WithGroundingPrompt(prompt string) RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		c.groundingPrompt = prompt
	})
}

// WithCitationStyle sets the citation format for GroundingCitation mode.
// This affects how citations appear in the model's responses.
//
// Available styles:
//   - CitationBracket: [1], [2] (default)
//   - CitationSuperscript: ⁽¹⁾, ⁽²⁾
//   - CitationParenthetical: (1), (2)
//
// Example:
//
//	ragAgent, _ := agent.NewRAG(model, retriever,
//	    agent.WithGroundingMode(agent.GroundingCitation),
//	    agent.WithCitationStyle(agent.CitationSuperscript),
//	)
func WithCitationStyle(style CitationStyle) RAGOption {
	return ragOptionFunc(func(c *ragOptions) {
		c.citationStyle = style
	})
}
