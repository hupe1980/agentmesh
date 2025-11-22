package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// ModelNode is a reusable graph node that generates responses using a language model.
// It implements the graph.Node interface and handles conversion between state and model inputs/outputs.
type ModelNode struct {
	name         string
	model        model.Model
	systemPrompt string
	tools        []tool.Tool
	outputSchema *schema.OutputSchema
}

// ModelNodeOption configures a ModelNode.
type ModelNodeOption func(*ModelNode)

// WithModelNodeName sets the name of the model node (default: "model").
func WithModelNodeName(name string) ModelNodeOption {
	return func(n *ModelNode) {
		n.name = name
	}
}

// WithModelSystemPrompt sets a system prompt for this model node.
// The system prompt is sent per-request and not stored in conversation state.
func WithModelSystemPrompt(prompt string) ModelNodeOption {
	return func(n *ModelNode) {
		n.systemPrompt = prompt
	}
}

// WithModelTools sets the tools available to the model for this node.
// The tools are passed to the model along with the request.
func WithModelTools(tools ...tool.Tool) ModelNodeOption {
	return func(n *ModelNode) {
		n.tools = tools
	}
}

// WithOutputSchema sets a structured output schema with metadata.
// The schema constrains the model to generate valid JSON matching the schema.
// Only works with models that support structured output (check Capabilities().StructuredOutput).
//
// This option provides better type safety and includes metadata like name, description, and strict mode.
// Model implementations can use the Strict flag, Description, and other metadata for provider-specific behavior.
//
// Example:
//
//	type AnalysisResult struct {
//	    Category   string  `json:"category" jsonschema:"required,description=The category"`
//	    Confidence float64 `json:"confidence" jsonschema:"required,description=Confidence score"`
//	}
//	outputSchema, _ := schema.NewOutputSchema("analysis", AnalysisResult{},
//	    schema.WithStrict(true),
//	    schema.WithDescription("Analysis result with category and confidence"))
//	node, err := NewModelNode(myModel, WithOutputSchema(&outputSchema))
func WithOutputSchema(outputSchema *schema.OutputSchema) ModelNodeOption {
	return func(n *ModelNode) {
		n.outputSchema = outputSchema
	}
}

// NewModelNode creates a reusable graph node that generates responses using the provided model.
// The node takes the current message history from the state and produces a new AI message.
//
// This component is commonly used in agent implementations to delegate response generation
// to a language model. It automatically handles the conversion between state and model inputs/outputs.
//
// Returns an error if the model parameter is nil.
//
// Example:
//
//	node, err := NewModelNode(myModel)
//	node, err := NewModelNode(myModel, WithModelNodeName("generator"))
//
// Plugins are automatically retrieved from context when the node executes.
func NewModelNode(mdl model.Model, opts ...ModelNodeOption) (*ModelNode, error) {
	if mdl == nil {
		return nil, fmt.Errorf("agent: model cannot be nil")
	}

	node := &ModelNode{
		name:  "model",
		model: mdl,
	}

	for _, opt := range opts {
		opt(node)
	}

	return node, nil
}

// Name returns the node's name.
func (n *ModelNode) Name() string {
	return n.name
}

// Execute runs the model node logic.
func (n *ModelNode) Execute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
	// Get messages from state
	messages := GetMessages(view)

	// Create request
	req := &model.Request{
		Messages:     messages,
		SystemPrompt: n.systemPrompt,
		Tools:        n.tools,
		OutputSchema: n.outputSchema,
	}

	// Execute BeforeModel plugins from context
	if pm := callbacks.FromContext(ctx); pm != nil && pm.HasPlugins() {
		resp, err := pm.ExecuteBeforeModel(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp != nil {
			// Short-circuit: use plugin response instead of calling model
			builder := state.NewUpdateBuilder()
			state.AppendUpdate(builder, MessagesKey, resp.Message)
			return builder.Build()
		}
	}

	// Observability: Create model call span
	tp := trace.FromContext(ctx)
	tracer := tp.Tracer("agentmesh.agent")
	modelName := n.name // Use node name as model identifier
	ctx, modelSpan := tracer.Start(ctx, "model.call",
		trace.Attr{Key: "model.node", Value: modelName},
		trace.Attr{Key: "model.messages", Value: len(messages)})
	var modelErr error
	defer func() {
		modelSpan.End(modelErr)
	}()

	// Observability: Log model call start
	logger := logging.FromContext(ctx)
	logger.Debug("model call starting", "model", modelName, "messages", len(messages))

	// Observability: Record model call metrics
	mp := metrics.FromContext(ctx)
	modelStartTime := time.Now()
	modelCallCounter := mp.Counter("model.requests")
	modelCallCounter.Add(ctx, 1, metrics.Attr{Key: "model", Value: modelName})

	// Call the model
	resp, err := model.Last(n.model.Generate(ctx, req))

	// Observability: Record metrics after call
	duration := time.Since(modelStartTime)
	modelDuration := mp.Histogram("model.duration_ms")
	modelDuration.Record(ctx, float64(duration.Milliseconds()),
		metrics.Attr{Key: "model", Value: modelName})

	if err != nil {
		modelErr = err
		// Record error metric
		modelErrors := mp.Counter("model.errors")
		modelErrors.Add(ctx, 1, metrics.Attr{Key: "model", Value: modelName})
		logger.Error("model call failed", "model", modelName, "error", err, "duration_ms", duration.Milliseconds())
		return n.handleModelError(ctx, req, err)
	}

	// Record token usage if available
	if resp.Usage != nil {
		tokensUsed := mp.Counter("model.tokens_used")
		tokensUsed.Add(ctx, float64(resp.Usage.TotalTokens),
			metrics.Attr{Key: "model", Value: modelName},
			metrics.Attr{Key: "type", Value: "total"})
		logger.Debug("model call completed", "model", modelName,
			"duration_ms", duration.Milliseconds(),
			"tokens_used", resp.Usage.TotalTokens,
			"prompt_tokens", resp.Usage.PromptTokens,
			"completion_tokens", resp.Usage.CompletionTokens)
	} else {
		logger.Debug("model call completed", "model", modelName, "duration_ms", duration.Milliseconds())
	}

	// Execute AfterModel plugins from context
	if pm := callbacks.FromContext(ctx); pm != nil && pm.HasPlugins() {
		transformed, err := pm.ExecuteAfterModel(ctx, req, resp)
		if err != nil {
			return nil, err
		}
		if transformed != nil {
			resp = transformed
		}
	}

	// Return message in updates map (agent layer handles message storage)
	builder := state.NewUpdateBuilder()
	state.AppendUpdate(builder, MessagesKey, resp.Message)

	return builder.Build()
}

// handleModelError processes model execution errors through plugins.
func (n *ModelNode) handleModelError(ctx context.Context, req *model.Request, err error) (state.Updates, error) {
	// Execute OnModelError plugins from context
	if pm := callbacks.FromContext(ctx); pm != nil && pm.HasPlugins() {
		fallback, transformedErr := pm.ExecuteOnModelError(ctx, req, err)
		if fallback != nil {
			// Plugin provided fallback response
			builder := state.NewUpdateBuilder()
			state.AppendUpdate(builder, MessagesKey, fallback.Message)
			return builder.Build()
		}
		if transformedErr != nil {
			return nil, transformedErr
		}
	}
	return nil, err
}
