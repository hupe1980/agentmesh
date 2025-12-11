package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sort"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/shared"
)

// Stream represents a streaming response from the OpenAI API.
type Stream interface {
	Next() bool
	Current() openai.ChatCompletionChunk
	Close() error
	Err() error
}

// Client defines the interface for interacting with the OpenAI API.
type Client interface {
	ChatCompletions(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	ChatCompletionsStreaming(ctx context.Context, req openai.ChatCompletionNewParams) Stream
}

// ClientWrapper wraps an OpenAI client to implement the Client interface.
type ClientWrapper struct {
	inner *openai.Client
}

// NewClientWrapper creates a new ClientWrapper.
// Returns an error if the client parameter is nil.
func NewClientWrapper(client *openai.Client) (*ClientWrapper, error) {
	if err := validate.NotNil(client, "openai: client"); err != nil {
		return nil, err
	}

	return &ClientWrapper{
		inner: client,
	}, nil
}

// ChatCompletions implements the ChatCompletions method of the Client interface.
func (c *ClientWrapper) ChatCompletions(
	ctx context.Context,
	req openai.ChatCompletionNewParams,
) (*openai.ChatCompletion, error) {
	return c.inner.Chat.Completions.New(ctx, req)
}

// ChatCompletionsStreaming implements the ChatCompletionsStreaming method of the Client interface.
func (c *ClientWrapper) ChatCompletionsStreaming(ctx context.Context, req openai.ChatCompletionNewParams) Stream {
	return c.inner.Chat.Completions.NewStreaming(ctx, req)
}

// Options configures OpenAI model behavior.
type Options struct {
	model               string
	temperature         float64
	maxCompletionTokens int64
}

// Model wraps the OpenAI API client for chat completion.
type Model struct {
	client Client
	model  string
	opts   Options
}

// NewModel creates a new OpenAI model with default client.
// This function is kept as non-error returning for backward compatibility since
// the default client construction cannot fail.
func NewModel(optFns ...func(o *Options)) *Model {
	client := openai.NewClient()
	model, _ := NewModelFromClient(&client, optFns...)
	return model
}

// NewModelFromClient creates a model from an existing OpenAI client.
// Returns an error if the client is nil.
func NewModelFromClient(client *openai.Client, optFns ...func(o *Options)) (*Model, error) {
	wrapper, err := NewClientWrapper(client)
	if err != nil {
		return nil, err
	}

	return NewModelFromClientWrapper(wrapper, optFns...)
}

// NewModelFromClientWrapper creates a model from a wrapped client.
// Returns an error if the wrapper is nil.
func NewModelFromClientWrapper(wrapper *ClientWrapper, optFns ...func(o *Options)) (*Model, error) {
	if err := validate.NotNil(wrapper, "openai: wrapper"); err != nil {
		return nil, err
	}

	opts := Options{
		model:               openai.ChatModelGPT4oMini,
		temperature:         0.7,
		maxCompletionTokens: 4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	modelName := opts.model
	if modelName == "" {
		modelName = openai.ChatModelGPT4oMini
	}

	return &Model{client: wrapper, model: modelName, opts: opts}, nil
}

// WithModel returns a new model configured to use the specified model name.
func WithModel(modelName string) func(o *Options) {
	return func(o *Options) {
		o.model = modelName
	}
}

// WithTemperature returns a new model with the specified temperature.
// Temperature controls randomness in the output (0.0 to 2.0).
func WithTemperature(temperature float64) func(o *Options) {
	return func(o *Options) {
		o.temperature = temperature
	}
}

// WithMaxCompletionTokens returns a new model with the specified maximum completion tokens.
func WithMaxCompletionTokens(maxTokens int64) func(o *Options) {
	return func(o *Options) {
		o.maxCompletionTokens = maxTokens
	}
}

// Name returns the configured OpenAI model identifier.
func (m *Model) Name() string {
	return m.model
}

// Capabilities returns the features and limitations of this OpenAI model.
func (m *Model) Capabilities() model.Capabilities {
	modelName := strings.ToLower(m.model)

	// Detect o1/o3 reasoning models
	isReasoningModel := strings.HasPrefix(modelName, "o1-") || strings.HasPrefix(modelName, "o3-")

	// Detect vision-capable models
	hasVision := strings.Contains(modelName, "vision") ||
		strings.Contains(modelName, "gpt-4o") ||
		strings.Contains(modelName, "gpt-4-turbo") ||
		(strings.HasPrefix(modelName, "gpt-4") && !strings.Contains(modelName, "gpt-4-"))

	// Context windows by model family
	contextWindow := m.getContextWindow(modelName)

	caps := model.Capabilities{
		Streaming:           true,
		Tools:               !isReasoningModel, // o1 doesn't support tools yet
		StructuredOutput:    true,
		NativeReasoning:     isReasoningModel,
		Logprobs:            !isReasoningModel, // o1 doesn't provide logprobs
		Vision:              hasVision,
		Audio:               false, // Not yet supported in this implementation
		MaxContextTokens:    contextWindow,
		MaxOutputTokens:     int(m.opts.maxCompletionTokens),
		SupportedModalities: m.getSupportedModalities(hasVision),
	}

	return caps
}

// getContextWindow returns the context window size for a given model.
func (m *Model) getContextWindow(modelName string) int {
	switch {
	case strings.HasPrefix(modelName, "gpt-4o"):
		return 128000
	case strings.HasPrefix(modelName, "gpt-4-turbo"), strings.HasPrefix(modelName, "gpt-4-1106"), strings.HasPrefix(modelName, "gpt-4-0125"):
		return 128000
	case strings.HasPrefix(modelName, "gpt-4-32k"):
		return 32768
	case strings.HasPrefix(modelName, "gpt-4"):
		return 8192
	case strings.HasPrefix(modelName, "gpt-3.5-turbo-16k"):
		return 16384
	case strings.HasPrefix(modelName, "gpt-3.5"):
		return 4096
	case strings.HasPrefix(modelName, "o1-"):
		return 128000
	case strings.HasPrefix(modelName, "o3-"):
		return 128000
	default:
		return 4096 // Conservative default
	}
}

// getSupportedModalities returns the list of input modalities.
func (m *Model) getSupportedModalities(hasVision bool) []string {
	if hasVision {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

// Generate executes a chat completion request against the OpenAI API.
// Returns an iterator that yields ModelResponse as they are received.
// For streaming, multiple intermediate responses are yielded followed by the final complete response.
// For non-streaming (blocking), only the final response is yielded.
func (m *Model) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		// Validate and prepare request
		params, err := m.prepareRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		// Handle streaming mode
		if req.Stream {
			if m.handleStreamingMode(ctx, params, yield) {
				return
			}
			// Fall through to blocking mode if streaming fails
		}

		// Handle blocking (non-streaming) mode
		m.handleBlockingMode(ctx, params, yield)
	}
}

// prepareRequest validates the request and builds OpenAI API parameters.
func (m *Model) prepareRequest(req *model.Request) (openai.ChatCompletionNewParams, error) {
	var params openai.ChatCompletionNewParams

	if req == nil || len(req.Messages) == 0 {
		return params, ErrNoMessages
	}

	// Prepare messages with optional instructions
	messages := m.prepareMessages(req)

	// Convert to OpenAI format
	converted, err := convertMessagesToOpenAI(messages)
	if err != nil {
		return params, err
	}

	params = openai.ChatCompletionNewParams{
		Model:    m.model,
		Messages: converted,
	}

	// Apply model options and request-specific settings
	if err := m.applyOptions(&params, req); err != nil {
		return params, err
	}

	return params, nil
}

// prepareMessages prepends instructions as system message if provided.
func (m *Model) prepareMessages(req *model.Request) []message.Message {
	messages := req.Messages
	if req.Instructions != "" {
		systemMsg := message.NewSystemMessageFromText(req.Instructions)
		messages = append([]message.Message{systemMsg}, messages...)
	}
	return messages
}

// handleStreamingMode handles streaming responses from OpenAI API.
// Returns true if streaming was handled (success or error), false if should fall through to blocking mode.
func (m *Model) handleStreamingMode(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	yield func(*model.Response, error) bool,
) bool {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	apiStream := m.client.ChatCompletionsStreaming(streamCtx, params)
	if apiStream.Err() != nil {
		return false // Fall through to blocking mode
	}

	m.streamGenerate(apiStream, yield, cancel)
	return true
}

// handleBlockingMode handles non-streaming responses from OpenAI API.
func (m *Model) handleBlockingMode(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	yield func(*model.Response, error) bool,
) {
	completion, err := m.client.ChatCompletions(ctx, params)
	if err != nil {
		yield(nil, err)
		return
	}

	if completion == nil || len(completion.Choices) == 0 {
		yield(nil, ErrNoChoices)
		return
	}

	response, err := m.buildBlockingResponse(completion)
	if err != nil {
		yield(nil, err)
		return
	}

	yield(response, nil)
}

// buildBlockingResponse constructs a model.Response from a completed API response.
func (m *Model) buildBlockingResponse(completion *openai.ChatCompletion) (*model.Response, error) {
	choice := completion.Choices[0]

	aiMessage := buildAIMessageFromChoice(&choice.Message)

	if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
		return nil, ErrEmptyMessage
	}

	response := &model.Response{
		Message:      aiMessage,
		FinishReason: choice.FinishReason,
		Usage: &model.UsageInfo{
			PromptTokens:     int(completion.Usage.PromptTokens),
			CompletionTokens: int(completion.Usage.CompletionTokens),
			TotalTokens:      int(completion.Usage.TotalTokens),
		},
		Partial: false,
	}

	// Populate logprobs if available
	if len(choice.Logprobs.Content) > 0 {
		response.Logprobs = convertLogprobs(choice.Logprobs)
	}

	return response, nil
}

// buildAIMessageFromChoice constructs an AIMessage from an OpenAI chat completion message.
func buildAIMessageFromChoice(msg *openai.ChatCompletionMessage) *message.AIMessage {
	text := extractMessageText(msg)

	var parts message.Parts
	if text != "" {
		parts = message.Parts{message.NewTextPart(text)}
	}

	aiMessage := message.NewAIMessage(parts)

	// Extract tool calls if present
	toolCalls := extractToolCalls(msg.ToolCalls)
	if len(toolCalls) > 0 {
		aiMessage.ToolCalls = toolCalls
	}

	return aiMessage
}

// extractMessageText extracts text content from an OpenAI message, falling back to refusal.
func extractMessageText(msg *openai.ChatCompletionMessage) string {
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		text = strings.TrimSpace(msg.Refusal)
	}
	return text
}

// extractToolCalls extracts tool calls from OpenAI message tool calls.
func extractToolCalls(openaiToolCalls []openai.ChatCompletionMessageToolCallUnion) []message.ToolCall {
	if len(openaiToolCalls) == 0 {
		return nil
	}

	toolCalls := make([]message.ToolCall, 0, len(openaiToolCalls))
	for idx := range openaiToolCalls {
		tc := &openaiToolCalls[idx]
		if tc.Type != "function" {
			continue
		}
		fn := tc.AsFunction()
		toolCalls = append(toolCalls, message.ToolCall{
			ID:        fn.ID,
			Name:      fn.Function.Name,
			Type:      string(fn.Type),
			Arguments: fn.Function.Arguments,
		})
	}
	return toolCalls
}

// streamGenerate handles streaming responses from OpenAI API
//
//nolint:gocyclo // Streaming requires handling many delta types and states
func (m *Model) streamGenerate(
	apiStream Stream,
	yield func(*model.Response, error) bool,
	cancel context.CancelFunc,
) {
	defer cancel()
	defer func() { _ = apiStream.Close() }() // Best effort close

	type toolCallAccumulator struct {
		id        string
		typ       string
		name      strings.Builder
		arguments strings.Builder
	}

	textBuilder := &strings.Builder{}
	toolCalls := make(map[int64]*toolCallAccumulator)
	var finishReason string

	for apiStream.Next() {
		chunk := apiStream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		delta := choice.Delta

		// Capture finish reason from the final chunk
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}

		if delta.Content != "" {
			textBuilder.WriteString(delta.Content)
			aiMessage := message.NewAIMessageFromText(delta.Content)
			response := &model.Response{
				Message: aiMessage,
				Partial: true, // Streaming chunk
			}
			if !yield(response, nil) {
				return
			}
		}

		if delta.Refusal != "" {
			textBuilder.WriteString(delta.Refusal)
			aiMessage := message.NewAIMessageFromText(delta.Refusal)
			response := &model.Response{
				Message: aiMessage,
				Partial: true, // Streaming chunk
			}
			if !yield(response, nil) {
				return
			}
		}

		//nolint:nestif // OpenAI SDK streaming delta handling, complexity is manageable
		if len(delta.ToolCalls) > 0 {
			for i := range delta.ToolCalls {
				tc := &delta.ToolCalls[i]
				acc, ok := toolCalls[tc.Index]
				if !ok {
					acc = &toolCallAccumulator{}
					toolCalls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Type != "" {
					acc.typ = tc.Type
				}
				if name := tc.Function.Name; name != "" {
					acc.name.WriteString(name)
				}
				if args := tc.Function.Arguments; args != "" {
					acc.arguments.WriteString(args)
				}
			}
		}
	}

	if err := apiStream.Err(); err != nil {
		yield(nil, err)
		return
	}

	finalText := strings.TrimSpace(textBuilder.String())
	var parts message.Parts
	if finalText != "" {
		parts = message.Parts{message.NewTextPart(finalText)}
	}

	aiMessage := message.NewAIMessage(parts)

	if len(toolCalls) > 0 {
		indices := make([]int, 0, len(toolCalls))
		for idx := range toolCalls {
			indices = append(indices, int(idx))
		}
		sort.Ints(indices)

		for _, idx := range indices {
			acc := toolCalls[int64(idx)]
			aiMessage.ToolCalls = append(aiMessage.ToolCalls, message.ToolCall{
				ID:        acc.id,
				Name:      acc.name.String(),
				Type:      acc.typ,
				Arguments: strings.TrimSpace(acc.arguments.String()),
			})
		}
	}

	if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
		yield(nil, ErrEmptyMessage)
		return
	}

	// Build final response
	response := &model.Response{
		Message:      aiMessage,
		FinishReason: finishReason,
		Partial:      false, // Final complete response
		// Note: Streaming doesn't provide usage information or logprobs in OpenAI API
		// Usage and Logprobs will be nil for streaming responses
	}

	yield(response, nil)
}

func (m *Model) applyOptions(params *openai.ChatCompletionNewParams, req *model.Request) error {
	if m == nil || params == nil {
		return nil
	}

	params.Temperature = param.NewOpt(m.opts.temperature)
	params.MaxCompletionTokens = param.NewOpt(m.opts.maxCompletionTokens)

	// Apply structured output from request if specified
	if req != nil && req.OutputSchema != nil {
		schema := req.OutputSchema.Schema
		// When strict mode is enabled, transform the schema to meet OpenAI's requirements:
		// - All properties must be in the required array
		// - Optional fields (not originally required) get nullable type: ["<type>", "null"]
		// - additionalProperties must be false (recursively)
		if req.OutputSchema.Strict {
			schema = transformSchemaForOpenAIStrict(schema)
		}

		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type: "json_schema",
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   req.OutputSchema.Name,
					Schema: schema,
					Strict: param.NewOpt(req.OutputSchema.Strict),
				},
			},
		}
	}

	// Apply tools from request if specified
	if req != nil && len(req.Tools) > 0 {
		converted, err := convertTools(normalizeTools(req.Tools))
		if err != nil {
			return err
		}
		if len(converted) > 0 {
			params.Tools = converted
		}
	}

	return nil
}

// transformSchemaForOpenAIStrict transforms a JSON schema to meet OpenAI's strict
// structured output requirements:
//   - All properties must be listed in the "required" array
//   - Properties that were originally optional get a nullable type: ["<type>", "null"]
//   - "additionalProperties" is set to false at all levels (recursively)
//
// This is necessary because OpenAI's Structured Output API has non-standard requirements
// where optional fields must still be in "required" but use a nullable type union.
// See: https://platform.openai.com/docs/guides/structured-outputs
func transformSchemaForOpenAIStrict(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	// Deep clone the schema to avoid mutating the original
	result := deepCloneMap(schema)

	// Process this schema level
	transformSchemaLevel(result)

	return result
}

// jsonNull is the JSON Schema null type constant.
const jsonNull = "null"

// transformSchemaLevel recursively transforms a schema object in place.
func transformSchemaLevel(schema map[string]any) {
	if schema == nil {
		return
	}

	// Set additionalProperties to false for objects
	if schemaType, ok := schema["type"]; ok && schemaType == "object" {
		schema["additionalProperties"] = false
	}

	// Get current required fields as a set for O(1) lookup
	requiredSet := buildRequiredSet(schema["required"])

	// Process properties
	if props, ok := schema["properties"].(map[string]any); ok {
		allPropertyNames := make([]string, 0, len(props))

		for propName, propValue := range props {
			allPropertyNames = append(allPropertyNames, propName)

			propSchema, ok := propValue.(map[string]any)
			if !ok {
				continue
			}

			// If this property was NOT originally required, make it nullable
			if !requiredSet[propName] {
				makeNullable(propSchema)
			}

			// Recursively transform nested schemas
			transformNestedSchemas(propSchema)
		}

		// Update required to include ALL property names
		schema["required"] = allPropertyNames
	}

	// Handle definitions/defs for schema references
	transformDefinitions(schema, "$defs")
	transformDefinitions(schema, "definitions")
}

// buildRequiredSet extracts required field names into a set for O(1) lookup.
func buildRequiredSet(required any) map[string]bool {
	result := make(map[string]bool)

	switch req := required.(type) {
	case []any:
		for _, r := range req {
			if s, ok := r.(string); ok {
				result[s] = true
			}
		}
	case []string:
		for _, r := range req {
			result[r] = true
		}
	}

	return result
}

// transformDefinitions processes $defs or definitions in a schema.
func transformDefinitions(schema map[string]any, key string) {
	if defs, ok := schema[key].(map[string]any); ok {
		for _, defValue := range defs {
			if defSchema, ok := defValue.(map[string]any); ok {
				transformSchemaLevel(defSchema)
			}
		}
	}
}

// transformNestedSchemas handles nested schema structures like items, anyOf, oneOf, allOf.
func transformNestedSchemas(schema map[string]any) {
	if schema == nil {
		return
	}

	// Handle array items
	if items, ok := schema["items"].(map[string]any); ok {
		transformSchemaLevel(items)
	}

	// Handle anyOf, oneOf, allOf
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := schema[key].([]any); ok {
			for _, item := range arr {
				if itemSchema, ok := item.(map[string]any); ok {
					transformSchemaLevel(itemSchema)
				}
			}
		}
	}

	// Handle nested object properties
	if schema["type"] == "object" {
		transformSchemaLevel(schema)
	}
}

// makeNullable converts a type to a nullable type union ["<type>", "null"].
// If the type is already nullable or is a union, it adds "null" to the union.
func makeNullable(schema map[string]any) {
	if schema == nil {
		return
	}

	currentType, hasType := schema["type"]
	if !hasType {
		makeNullableComposite(schema)
		return
	}

	makeNullableType(schema, currentType)
}

// makeNullableComposite handles nullable conversion for schemas using anyOf, oneOf, or $ref.
func makeNullableComposite(schema map[string]any) {
	if arr, ok := schema["anyOf"].([]any); ok {
		if !containsNullType(arr) {
			schema["anyOf"] = append(arr, map[string]any{"type": jsonNull})
		}
		return
	}

	if arr, ok := schema["oneOf"].([]any); ok {
		delete(schema, "oneOf")
		schema["anyOf"] = append(arr, map[string]any{"type": jsonNull})
		return
	}

	if ref, hasRef := schema["$ref"]; hasRef {
		delete(schema, "$ref")
		schema["anyOf"] = []any{
			map[string]any{"$ref": ref},
			map[string]any{"type": jsonNull},
		}
	}
}

// makeNullableType handles nullable conversion for schemas with explicit type.
func makeNullableType(schema map[string]any, currentType any) {
	switch t := currentType.(type) {
	case string:
		if t != jsonNull {
			schema["type"] = []any{t, jsonNull}
		}
	case []any:
		if !containsNull(t) {
			schema["type"] = append(t, jsonNull)
		}
	case []string:
		if !containsNullString(t) {
			newType := make([]any, len(t)+1)
			for i, item := range t {
				newType[i] = item
			}
			newType[len(t)] = jsonNull
			schema["type"] = newType
		}
	}
}

// containsNullType checks if an anyOf/oneOf array contains a null type schema.
func containsNullType(arr []any) bool {
	for _, item := range arr {
		if itemMap, ok := item.(map[string]any); ok {
			if itemMap["type"] == jsonNull {
				return true
			}
		}
	}
	return false
}

// containsNull checks if an []any type array contains "null".
func containsNull(arr []any) bool {
	for _, item := range arr {
		if item == jsonNull {
			return true
		}
	}
	return false
}

// containsNullString checks if a []string type array contains "null".
func containsNullString(arr []string) bool {
	for _, item := range arr {
		if item == jsonNull {
			return true
		}
	}
	return false
}

// deepCloneMap creates a deep copy of a map[string]any.
func deepCloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = deepCloneValue(v)
	}
	return result
}

// deepCloneValue creates a deep copy of any value.
func deepCloneValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCloneMap(val)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = deepCloneValue(item)
		}
		return result
	case []string:
		result := make([]string, len(val))
		copy(result, val)
		return result
	default:
		// Primitive types (string, int, float, bool, nil) are immutable
		return val
	}
}

func convertTools(tools []tool.Tool) ([]openai.ChatCompletionToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	converted := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for idx, tool := range tools {
		if tool == nil {
			continue
		}
		definition := tool.Definition()
		if definition == nil {
			continue
		}
		if definition.Type != "" && definition.Type != "function" {
			return nil, fmt.Errorf("openai model: unsupported tool type %q", definition.Type)
		}

		fn := definition.Function
		if fn.Name == "" {
			return nil, fmt.Errorf("openai model: tool at index %d missing function name", idx)
		}

		function := shared.FunctionDefinitionParam{
			Name: fn.Name,
		}
		if fn.Description != "" {
			function.Description = param.NewOpt(fn.Description)
		}
		if len(fn.Parameters) > 0 {
			function.Parameters = shared.FunctionParameters(fn.Parameters)
		}

		converted = append(converted, openai.ChatCompletionFunctionTool(function))
	}

	return converted, nil
}

func convertMessagesToOpenAI(messages []message.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for idx, msg := range messages {
		if err := validate.NotNil(msg, fmt.Sprintf("messages[%d]", idx)); err != nil {
			return nil, err
		}

		text, err := joinTextParts(msg.Parts())
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", idx, err)
		}

		var converted openai.ChatCompletionMessageParamUnion
		switch msg.Type() {
		case message.TypeSystem:
			converted = openai.SystemMessage(text)
		case message.TypeHuman:
			converted = openai.UserMessage(text)
		case message.TypeAI:
			aiMsg, ok := msg.(*message.AIMessage)
			if !ok {
				return nil, fmt.Errorf("messages[%d]: expected *message.AIMessage for ai type", idx)
			}

			assistant := openai.ChatCompletionAssistantMessageParam{}
			if text != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(text),
				}
			}

			toolCalls, err := convertToolCalls(aiMsg.ToolCalls)
			if err != nil {
				return nil, fmt.Errorf("messages[%d]: %w", idx, err)
			}
			if len(toolCalls) > 0 {
				assistant.ToolCalls = toolCalls
			}

			converted = openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}
		case message.TypeTool:
			toolMsg, ok := msg.(*message.ToolMessage)
			if !ok {
				return nil, fmt.Errorf("messages[%d]: expected *message.ToolMessage for tool type", idx)
			}
			converted = openai.ToolMessage(text, toolMsg.ToolCallID)
		default:
			return nil, fmt.Errorf("unsupported message type %q", msg.Type())
		}
		result = append(result, converted)
	}
	return result, nil
}

func convertToolCalls(calls []message.ToolCall) ([]openai.ChatCompletionMessageToolCallUnionParam, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for idx, call := range calls {
		if call.Name == "" {
			return nil, fmt.Errorf("tool call[%d]: missing name", idx)
		}

		arguments := "{}"
		if call.Arguments != "" {
			payload, err := json.Marshal(call.Arguments)
			if err != nil {
				return nil, fmt.Errorf("tool call[%d]: marshal arguments: %w", idx, err)
			}
			arguments = string(payload)
		}

		callID := call.ID
		if callID == "" {
			callID = fmt.Sprintf("%s-%d", call.Name, idx)
		}

		toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: callID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      call.Name,
					Arguments: arguments,
				},
			},
		})
	}
	return toolCalls, nil
}

func normalizeTools(tools []tool.Tool) []tool.Tool {
	if len(tools) == 0 {
		return nil
	}

	dedup := make([]tool.Tool, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))

	for _, tool := range tools {
		if tool == nil {
			continue
		}

		name := tool.Name()
		if name != "" {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
		}

		dedup = append(dedup, tool)
	}

	if len(dedup) == 0 {
		return nil
	}

	return append([]tool.Tool(nil), dedup...)
}

func joinTextParts(parts message.Parts) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for i, part := range parts {
		switch p := part.(type) {
		case message.TextPart:
			sb.WriteString(p.Text)
		case *message.TextPart:
			if p != nil {
				sb.WriteString(p.Text)
			}
		default:
			return "", fmt.Errorf("unsupported part type %T", part)
		}
		if i < len(parts)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// convertLogprobs converts OpenAI logprobs to agentmesh logprobs format
func convertLogprobs(openaiLogprobs openai.ChatCompletionChoiceLogprobs) *model.Logprobs {
	if len(openaiLogprobs.Content) == 0 {
		return nil
	}

	content := make([]model.TokenLogprob, 0, len(openaiLogprobs.Content))
	for i := range openaiLogprobs.Content {
		item := &openaiLogprobs.Content[i]
		tokenLogprob := model.TokenLogprob{
			Token:   item.Token,
			Logprob: item.Logprob,
		}

		// Convert bytes if available (OpenAI uses []int64, we use []byte)
		if len(item.Bytes) > 0 {
			bytes := make([]byte, len(item.Bytes))
			for j, b := range item.Bytes {
				bytes[j] = byte(b)
			}
			tokenLogprob.Bytes = bytes
		}

		// Convert top logprobs if available
		if len(item.TopLogprobs) > 0 {
			topLogprobs := make([]model.TopLogprob, 0, len(item.TopLogprobs))
			for j := range item.TopLogprobs {
				top := &item.TopLogprobs[j]
				topLogprob := model.TopLogprob{
					Token:   top.Token,
					Logprob: top.Logprob,
				}
				if len(top.Bytes) > 0 {
					bytes := make([]byte, len(top.Bytes))
					for k, b := range top.Bytes {
						bytes[k] = byte(b)
					}
					topLogprob.Bytes = bytes
				}
				topLogprobs = append(topLogprobs, topLogprob)
			}
			tokenLogprob.TopLogprobs = topLogprobs
		}

		content = append(content, tokenLogprob)
	}

	return &model.Logprobs{
		Content: content,
	}
}

// Compile-time interface checks
var _ model.Model = (*Model)(nil)
