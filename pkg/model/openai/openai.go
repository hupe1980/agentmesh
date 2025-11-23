package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sort"
	"strings"

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
	if client == nil {
		return nil, fmt.Errorf("openai: client cannot be nil")
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
	if wrapper == nil {
		return nil, fmt.Errorf("openai: wrapper cannot be nil")
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
//
//nolint:gocyclo // Generation requires handling many message and response types
func (m *Model) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		if req == nil || len(req.Messages) == 0 {
			yield(nil, fmt.Errorf("generate requires at least one message"))
			return
		}

		messages := req.Messages

		// Prepend system prompt if provided (per-request)
		if req.SystemPrompt != "" {
			systemMsg := message.NewSystemMessageFromText(req.SystemPrompt)
			messages = append([]message.Message{systemMsg}, messages...)
		}

		converted, err := convertMessagesToOpenAI(messages)
		if err != nil {
			yield(nil, err)
			return
		}

		params := openai.ChatCompletionNewParams{
			Model:    m.model,
			Messages: converted,
		}

		if err := m.applyOptions(&params, req); err != nil {
			yield(nil, err)
			return
		}

		// Choose streaming or non-streaming based on request
		if req.Stream {
			streamCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			apiStream := m.client.ChatCompletionsStreaming(streamCtx, params)
			if apiStream.Err() == nil {
				// Streaming successful
				m.streamGenerate(apiStream, yield, cancel)
				return
			}
			// If streaming fails, fall through to non-streaming
		}

		// Non-streaming mode
		completion, err := m.client.ChatCompletions(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		if completion == nil || len(completion.Choices) == 0 {
			yield(nil, fmt.Errorf("openai chat completion returned no choices"))
			return
		}

		choice := completion.Choices[0]
		text := strings.TrimSpace(choice.Message.Content)
		if text == "" {
			text = strings.TrimSpace(choice.Message.Refusal)
		}

		var parts message.Parts
		if text != "" {
			parts = message.Parts{message.NewTextPart(text)}
		}

		aiMessage := message.NewAIMessage(parts)

		if len(choice.Message.ToolCalls) > 0 {
			toolCalls := make([]message.ToolCall, 0, len(choice.Message.ToolCalls))
			for idx := range choice.Message.ToolCalls {
				if choice.Message.ToolCalls[idx].Type != "function" {
					continue
				}
				fn := choice.Message.ToolCalls[idx].AsFunction()
				toolCalls = append(toolCalls, message.ToolCall{
					ID:        fn.ID,
					Name:      fn.Function.Name,
					Type:      string(fn.Type),
					Arguments: fn.Function.Arguments,
				})
			}
			if len(toolCalls) > 0 {
				aiMessage.ToolCalls = toolCalls
			}
		}

		if len(aiMessage.Parts()) == 0 && len(aiMessage.ToolCalls) == 0 {
			yield(nil, fmt.Errorf("openai chat completion returned empty message"))
			return
		}

		// Build ModelResponse with usage information
		response := &model.Response{
			Message:      aiMessage,
			FinishReason: choice.FinishReason,
			Usage: &model.UsageInfo{
				PromptTokens:     int(completion.Usage.PromptTokens),
				CompletionTokens: int(completion.Usage.CompletionTokens),
				TotalTokens:      int(completion.Usage.TotalTokens),
			},
			Partial: false, // Blocking mode: single complete response
		}

		// Populate logprobs if available
		if len(choice.Logprobs.Content) > 0 {
			response.Logprobs = convertLogprobs(choice.Logprobs)
		}

		// Note: OpenAI o1/o3 models would expose reasoning_content here
		// This will be added when implementing o1-specific support

		yield(response, nil)
	}
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
		yield(nil, fmt.Errorf("openai chat completion returned empty message"))
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
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type: "json_schema",
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   req.OutputSchema.Name,
					Schema: req.OutputSchema.Schema,
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
		if msg == nil {
			return nil, fmt.Errorf("messages[%d]: message must not be nil", idx)
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
