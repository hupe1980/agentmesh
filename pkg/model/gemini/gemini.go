package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"runtime"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"google.golang.org/genai"
)

// Client defines the interface for interacting with the Gemini API.
type Client interface {
	GenerateContent(ctx context.Context, modelName string, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
	GenerateContentStream(ctx context.Context, modelName string, contents []*genai.Content, cfg *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error]
}

// ClientWrapper wraps the Gemini SDK client to implement the Client interface.
type ClientWrapper struct {
	inner *genai.Client
}

// NewClientWrapper creates a new ClientWrapper.
// Returns an error if the client parameter is nil.
func NewClientWrapper(client *genai.Client) (*ClientWrapper, error) {
	if err := validate.NotNil(client, "gemini: client"); err != nil {
		return nil, err
	}

	return &ClientWrapper{inner: client}, nil
}

// GenerateContent implements the GenerateContent method of the Client interface.
func (c *ClientWrapper) GenerateContent(
	ctx context.Context,
	modelName string,
	contents []*genai.Content,
	cfg *genai.GenerateContentConfig,
) (*genai.GenerateContentResponse, error) {
	return c.inner.Models.GenerateContent(ctx, modelName, contents, cfg)
}

// GenerateContentStream implements the GenerateContentStream method of the Client interface.
func (c *ClientWrapper) GenerateContentStream(
	ctx context.Context,
	modelName string,
	contents []*genai.Content,
	cfg *genai.GenerateContentConfig,
) iter.Seq2[*genai.GenerateContentResponse, error] {
	return c.inner.Models.GenerateContentStream(ctx, modelName, contents, cfg)
}

// Options configures the Gemini model.
type Options struct {
	model              string
	temperature        float32
	maxOutputTokens    int32
	topP               float32
	topK               float32
	apiKey             string
	versionHeaderValue string
}

// Model implements the model.Model interface for Google Gemini.
type Model struct {
	client Client
	opts   Options
}

// NewModel creates a new Gemini model with the given options.
func NewModel(ctx context.Context, optFns ...func(o *Options)) (*Model, error) {
	opts := Options{
		model:           "gemini-2.0-flash-exp",
		temperature:     0.7,
		maxOutputTokens: 4096,
		topP:            0.95,
		topK:            40,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	clientCfg := &genai.ClientConfig{}
	if opts.apiKey != "" {
		clientCfg.APIKey = opts.apiKey
	}

	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	wrapper, err := NewClientWrapper(client)
	if err != nil {
		return nil, err
	}

	return &Model{
		client: wrapper,
		opts:   opts,
	}, nil
}

// NewModelFromClient creates a model from a custom client (for testing).
// Returns an error if the client is nil.
func NewModelFromClient(client Client, optFns ...func(o *Options)) (*Model, error) {
	if err := validate.NotNil(client, "gemini: client"); err != nil {
		return nil, err
	}

	opts := Options{
		model:           "gemini-2.0-flash-exp",
		temperature:     0.7,
		maxOutputTokens: 4096,
		topP:            0.95,
		topK:            40,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	// Create version header value
	opts.versionHeaderValue = fmt.Sprintf("agentmesh gl-go/%s",
		strings.TrimPrefix(runtime.Version(), "go"))

	return &Model{
		client: client,
		opts:   opts,
	}, nil
}

// WithModel returns an option function to set the model name.
func WithModel(modelName string) func(o *Options) {
	return func(o *Options) {
		o.model = modelName
	}
}

// WithTemperature returns an option function to set the temperature.
// Temperature controls randomness in the output.
func WithTemperature(temperature float32) func(o *Options) {
	return func(o *Options) {
		o.temperature = temperature
	}
}

// WithMaxOutputTokens returns an option function to set the maximum output tokens.
func WithMaxOutputTokens(maxTokens int32) func(o *Options) {
	return func(o *Options) {
		o.maxOutputTokens = maxTokens
	}
}

// WithTopP returns an option function to set the top-p sampling parameter.
func WithTopP(topP float32) func(o *Options) {
	return func(o *Options) {
		o.topP = topP
	}
}

// WithTopK returns an option function to set the top-k sampling parameter.
func WithTopK(topK float32) func(o *Options) {
	return func(o *Options) {
		o.topK = topK
	}
}

// WithAPIKey returns an option function to set the API key.
func WithAPIKey(apiKey string) func(o *Options) {
	return func(o *Options) {
		o.apiKey = apiKey
	}
}

// Name returns the configured Gemini model identifier.
func (m *Model) Name() string {
	return m.opts.model
}

// Capabilities returns the features and limitations of this Gemini model.
func (m *Model) Capabilities() model.Capabilities {
	modelName := strings.ToLower(m.opts.model)

	// Gemini 2.0 Flash supports thinking mode (native reasoning)
	hasThinkingMode := strings.Contains(modelName, "gemini-2.0") || strings.Contains(modelName, "gemini-exp-1206")

	// Most Gemini models support vision
	hasVision := !strings.Contains(modelName, "text-only")

	// Context window varies by model
	contextWindow := m.getContextWindow(modelName)

	caps := model.Capabilities{
		Streaming:           true,
		Tools:               true,  // All Gemini models support function calling
		StructuredOutput:    false, // Gemini doesn't have built-in JSON schema validation
		NativeReasoning:     hasThinkingMode,
		Logprobs:            false, // Gemini doesn't provide logprobs
		Vision:              hasVision,
		Audio:               false, // Audio support not yet implemented
		MaxContextTokens:    contextWindow,
		MaxOutputTokens:     int(m.opts.maxOutputTokens),
		SupportedModalities: m.getSupportedModalities(hasVision),
	}

	return caps
}

// getContextWindow returns the context window size for a given Gemini model.
func (m *Model) getContextWindow(modelName string) int {
	switch {
	case strings.Contains(modelName, "gemini-2.0"):
		return 1000000 // 1M token context for Gemini 2.0
	case strings.Contains(modelName, "gemini-1.5-pro"), strings.Contains(modelName, "gemini-1.5-flash"):
		return 1000000 // 1M token context for Gemini 1.5
	case strings.Contains(modelName, "gemini-pro"):
		return 32768
	default:
		return 32768 // Conservative default
	}
}

// getSupportedModalities returns the list of input modalities.
func (m *Model) getSupportedModalities(hasVision bool) []string {
	if hasVision {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

// Generate executes a content generation request against the Gemini API.
// Returns an iterator that yields ModelResponse as they are received.
// For streaming, multiple intermediate responses are yielded followed by the final complete response.
// For non-streaming (blocking), only the final response is yielded.
func (m *Model) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		if req == nil || len(req.Messages) == 0 {
			yield(nil, ErrNoMessages)
			return
		}

		contents, systemInstruction := convertMessagesToGemini(req.Messages)

		// Use request instructions if provided, otherwise use extracted system instruction
		finalInstructions := systemInstruction
		if req.Instructions != "" {
			// Combine: request instructions + extracted system instruction
			if systemInstruction != "" {
				finalInstructions = req.Instructions + "\n\n" + systemInstruction
			} else {
				finalInstructions = req.Instructions
			}
		}

		cfg := m.buildConfig(finalInstructions, req)

		// Choose streaming or non-streaming based on request
		if req.Stream {
			m.streamGenerate(ctx, &contents, cfg, yield)
		} else {
			m.blockingGenerate(ctx, &contents, cfg, yield)
		}
	}
}

// streamGenerate handles streaming responses from Gemini API
//
//nolint:gocyclo // Streaming requires handling many response types
func (m *Model) streamGenerate(
	ctx context.Context,
	contents *[]*genai.Content,
	cfg *genai.GenerateContentConfig,
	yield func(*model.Response, error) bool,
) {
	// Ensure user content as last message (Gemini requirement)
	m.maybeAppendUserContent(contents)

	textBuilder := &strings.Builder{}
	var toolCalls []message.ToolCall
	var finishReason string

	for resp, err := range m.client.GenerateContentStream(ctx, m.opts.model, *contents, cfg) {
		if err != nil {
			yield(nil, err)
			return
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		candidate := resp.Candidates[0]

		// Capture finish reason if available
		if candidate.FinishReason != genai.FinishReasonUnspecified {
			finishReason = string(candidate.FinishReason)
		}

		if candidate.Content == nil {
			continue
		}

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text := part.Text
				textBuilder.WriteString(text)
				aiMsg := message.NewAIMessageFromText(text)
				response := &model.Response{
					Message: aiMsg,
					Partial: true, // Streaming chunk
				}
				if !yield(response, nil) {
					return
				}
			}

			if part.FunctionCall != nil {
				// Accumulate tool calls for final message
				// Marshal args to JSON string
				argsJSON := "{}"
				if part.FunctionCall.Args != nil {
					if b, err := json.Marshal(part.FunctionCall.Args); err == nil {
						argsJSON = string(b)
					}
				}
				toolCalls = append(toolCalls, message.ToolCall{
					ID:        part.FunctionCall.Name, // Gemini uses name as ID
					Name:      part.FunctionCall.Name,
					Type:      "function",
					Arguments: argsJSON,
				})
			}
		}
	}

	// Send final message with accumulated content
	finalText := strings.TrimSpace(textBuilder.String())
	var parts message.Parts
	if finalText != "" {
		parts = message.Parts{message.NewTextPart(finalText)}
	}

	aiMessage := message.NewAIMessage(parts)
	if len(toolCalls) > 0 {
		aiMessage.ToolCalls = toolCalls
	}

	if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
		yield(nil, ErrNoContent)
		return
	}

	// Build final response
	response := &model.Response{
		Message:      aiMessage,
		FinishReason: finishReason,
		Partial:      false, // Final complete response
		// Note: Gemini 2.0 Flash with thinking mode would populate Reasoning here
		// Note: Usage information and logprobs not available in streaming mode
	}

	yield(response, nil)
}

// blockingGenerate handles non-streaming responses from Gemini API
func (m *Model) blockingGenerate(
	ctx context.Context,
	contents *[]*genai.Content,
	cfg *genai.GenerateContentConfig,
	yield func(*model.Response, error) bool,
) {
	// Ensure user content as last message (Gemini requirement)
	m.maybeAppendUserContent(contents)

	resp, err := m.client.GenerateContent(ctx, m.opts.model, *contents, cfg)
	if err != nil {
		yield(nil, err)
		return
	}

	if len(resp.Candidates) == 0 {
		yield(nil, ErrNoCandidates)
		return
	}

	candidate := resp.Candidates[0]

	// Extract finish reason
	finishReason := ""
	if candidate.FinishReason != genai.FinishReasonUnspecified {
		finishReason = string(candidate.FinishReason)
	}

	if candidate.Content == nil {
		yield(nil, ErrNoContent)
		return
	}

	var textParts []string
	var toolCalls []message.ToolCall

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}

		if part.FunctionCall != nil {
			// Marshal args to JSON string
			argsJSON := "{}"
			if part.FunctionCall.Args != nil {
				if b, err := json.Marshal(part.FunctionCall.Args); err == nil {
					argsJSON = string(b)
				}
			}
			toolCalls = append(toolCalls, message.ToolCall{
				ID:        part.FunctionCall.Name, // Gemini uses name as ID
				Name:      part.FunctionCall.Name,
				Type:      "function",
				Arguments: argsJSON,
			})
		}
	}

	// Build message
	var parts message.Parts
	finalText := strings.TrimSpace(strings.Join(textParts, ""))
	if finalText != "" {
		parts = message.Parts{message.NewTextPart(finalText)}
	}

	aiMessage := message.NewAIMessage(parts)
	if len(toolCalls) > 0 {
		aiMessage.ToolCalls = toolCalls
	}

	if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
		yield(nil, ErrNoContent)
		return
	}

	// Build response
	response := &model.Response{
		Message:      aiMessage,
		FinishReason: finishReason,
		Partial:      false,
	}

	yield(response, nil)
}

// buildConfig creates the Gemini generation config with tools and settings
func (m *Model) buildConfig(systemInstruction string, req *model.Request) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{
		Temperature:     &m.opts.temperature,
		MaxOutputTokens: m.opts.maxOutputTokens,
		TopP:            &m.opts.topP,
		TopK:            &m.opts.topK,
	}

	if systemInstruction != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: systemInstruction}},
		}
	}

	// Apply tools from request if specified
	if req != nil && len(req.Tools) > 0 {
		cfg.Tools = []*genai.Tool{convertToolsToGemini(normalizeTools(req.Tools))}
	}

	return cfg
}

// maybeAppendUserContent ensures the last content is from user (Gemini requirement)
func (m *Model) maybeAppendUserContent(contents *[]*genai.Content) {
	if len(*contents) == 0 {
		return
	}

	if last := (*contents)[len(*contents)-1]; last != nil && last.Role != "user" {
		*contents = append(*contents, &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: "Continue processing previous requests as instructed."}},
		})
	}
}

// Helper functions

func normalizeTools(tools []tool.Tool) []tool.Tool {
	if len(tools) == 0 {
		return nil
	}

	dedup := make([]tool.Tool, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))

	for _, t := range tools {
		if t == nil {
			continue
		}

		name := t.Name()
		if name != "" {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
		}

		dedup = append(dedup, t)
	}

	if len(dedup) == 0 {
		return nil
	}

	return append([]tool.Tool(nil), dedup...)
}

//nolint:gocyclo // Message conversion requires handling many message types
func convertMessagesToGemini(msgs []message.Message) ([]*genai.Content, string) {
	var contents []*genai.Content
	var systemInstruction string

	for _, msg := range msgs {
		switch msg.Type() {
		case message.TypeSystem:
			// Extract system instruction
			parts := msg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					systemInstruction = textPart.Text
				}
			}

		case message.TypeHuman:
			var gParts []*genai.Part
			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					gParts = append(gParts, &genai.Part{Text: textPart.Text})
				}
			}
			if len(gParts) > 0 {
				contents = append(contents, &genai.Content{
					Role:  "user",
					Parts: gParts,
				})
			}

		case message.TypeAI:
			aiMsg, ok := msg.(*message.AIMessage)
			if !ok {
				continue
			}

			var gParts []*genai.Part
			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					gParts = append(gParts, &genai.Part{Text: textPart.Text})
				}
			}

			// Add tool calls
			for _, tc := range aiMsg.ToolCalls {
				// Unmarshal Arguments string to map for Gemini
				var args map[string]any
				if tc.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Arguments), &args)
				}
				gParts = append(gParts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: tc.Name,
						Args: args,
					},
				})
			}

			if len(gParts) > 0 {
				contents = append(contents, &genai.Content{
					Role:  "model",
					Parts: gParts,
				})
			}

		case message.TypeTool:
			toolMsg, ok := msg.(*message.ToolMessage)
			if !ok {
				continue
			}

			var resultText string
			parts := toolMsg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					resultText = textPart.Text
				}
			}
			if resultText == "" {
				resultText = fmt.Sprintf("%v", parts)
			}

			// Gemini expects function responses in user role
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: toolMsg.ToolCallID,
							Response: map[string]any{
								"result": resultText,
							},
						},
					},
				},
			})
		}
	}

	return contents, systemInstruction
}

// convertParametersToGeminiSchema converts tool parameters to Gemini schema format.
func convertParametersToGeminiSchema(parameters map[string]any) *genai.Schema {
	if len(parameters) == 0 {
		return nil
	}

	schemaJSON, err := json.Marshal(parameters)
	if err != nil {
		return nil
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(schemaJSON, &schemaMap); err != nil {
		return nil
	}

	schema := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: convertPropertiesToGemini(schemaMap),
	}

	// Extract required fields
	if required, ok := schemaMap["required"].([]any); ok {
		requiredFields := make([]string, 0, len(required))
		for _, r := range required {
			if str, ok := r.(string); ok {
				requiredFields = append(requiredFields, str)
			}
		}
		schema.Required = requiredFields
	}

	return schema
}

func convertToolsToGemini(tools []tool.Tool) *genai.Tool {
	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, t := range tools {
		if t == nil {
			continue
		}

		def := t.Definition()
		if def == nil {
			continue
		}

		fn := def.Function

		// Convert parameters to Gemini schema format
		schema := convertParametersToGeminiSchema(fn.Parameters)

		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:        fn.Name,
			Description: fn.Description,
			Parameters:  schema,
		})
	}

	return &genai.Tool{
		FunctionDeclarations: declarations,
	}
}

func convertPropertiesToGemini(schemaMap map[string]any) map[string]*genai.Schema {
	properties := make(map[string]*genai.Schema)

	propsMap, ok := schemaMap["properties"].(map[string]any)
	if !ok {
		return properties
	}

	for name, propDef := range propsMap {
		propMap, ok := propDef.(map[string]any)
		if !ok {
			continue
		}

		schema := &genai.Schema{}

		// Convert type
		if typeStr, ok := propMap["type"].(string); ok {
			schema.Type = convertTypeToGemini(typeStr)
		}

		// Add description
		if desc, ok := propMap["description"].(string); ok {
			schema.Description = desc
		}

		properties[name] = schema
	}

	return properties
}

func convertTypeToGemini(typeStr string) genai.Type {
	switch typeStr {
	case "string":
		return genai.TypeString
	case "integer", "number":
		return genai.TypeNumber
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case "object":
		return genai.TypeObject
	default:
		return genai.TypeString
	}
}

// Compile-time interface checks
var _ model.Model = (*Model)(nil)
