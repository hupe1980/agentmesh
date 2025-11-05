package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// Client defines the interface for interacting with the Anthropic API.
type Client interface {
	Messages() *anthropic.MessageService
}

// ClientWrapper wraps the Anthropic SDK client.
type ClientWrapper struct {
	inner *anthropic.Client
}

// NewClientWrapper creates a new ClientWrapper.
func NewClientWrapper(client *anthropic.Client) *ClientWrapper {
	return &ClientWrapper{inner: client}
}

// Messages returns the messages service.
func (c *ClientWrapper) Messages() *anthropic.MessageService {
	return &c.inner.Messages
}

// Options configures the Anthropic model.
type Options struct {
	Model       string
	MaxTokens   int64
	Temperature float64
	APIKey      string
}

// Model implements the model.Model interface for Anthropic Claude.
type Model struct {
	client Client
	opts   Options
	tools  []tool.Tool
}

// NewModel creates a new Anthropic model with the given options.
func NewModel(optFns ...func(o *Options)) *Model {
	opts := Options{
		Model:       string(anthropic.ModelClaudeSonnet4_0),
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	clientOpts := []option.RequestOption{}
	if opts.APIKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.APIKey))
	}

	client := anthropic.NewClient(clientOpts...)

	return &Model{
		client: &ClientWrapper{inner: &client},
		opts:   opts,
	}
}

// NewModelFromClient creates a model from a custom client (for testing).
func NewModelFromClient(client Client, optFns ...func(o *Options)) *Model {
	opts := Options{
		Model:       string(anthropic.ModelClaudeSonnet4_0),
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Model{
		client: client,
		opts:   opts,
	}
}

// BindTools returns a copy of the model configured with the provided tools.
func (m *Model) BindTools(tools ...tool.Tool) model.Model {
	if m == nil {
		return nil
	}

	clone := *m
	clone.tools = normalizeTools(tools)

	return &clone
}

// Generate executes a message request against the Anthropic API.
func (m *Model) Generate(ctx context.Context, msgs []message.Message) (message.Message, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("generate requires at least one message")
	}

	converted, systemText, err := convertMessagesToAnthropic(msgs)
	if err != nil {
		return nil, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(m.opts.Model),
		Messages:  converted,
		MaxTokens: m.opts.MaxTokens,
	}

	if systemText != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemText},
		}
	}

	if m.opts.Temperature > 0 {
		params.Temperature = param.NewOpt(m.opts.Temperature)
	}

	if len(m.tools) > 0 {
		params.Tools = convertToolsToAnthropic(m.tools)
	}

	response, err := m.client.Messages().New(ctx, params)
	if err != nil {
		return nil, err
	}

	return convertAnthropicResponseToMessage(response)
}

// Stream implements incremental streaming for message generation.
func (m *Model) Stream(ctx context.Context, msgs []message.Message) (*model.Stream, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("stream requires at least one message")
	}

	converted, systemText, err := convertMessagesToAnthropic(msgs)
	if err != nil {
		return nil, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(m.opts.Model),
		Messages:  converted,
		MaxTokens: m.opts.MaxTokens,
	}

	if systemText != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemText},
		}
	}

	if m.opts.Temperature > 0 {
		params.Temperature = param.NewOpt(m.opts.Temperature)
	}

	if len(m.tools) > 0 {
		params.Tools = convertToolsToAnthropic(m.tools)
	}

	stream := m.client.Messages().NewStreaming(ctx, params)

	return newStreamFromAnthropic(stream), nil
}

// Helper functions

func normalizeTools(tools []tool.Tool) []tool.Tool {
	if tools == nil {
		return nil
	}
	result := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if t != nil {
			result = append(result, t)
		}
	}
	return result
}

func convertMessagesToAnthropic(msgs []message.Message) ([]anthropic.MessageParam, string, error) {
	var apiMessages []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range msgs {
		switch msg.Type() {
		case message.TypeSystem:
			parts := msg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					systemPrompt = textPart.Text
				}
			}

		case message.TypeHuman:
			var content []anthropic.ContentBlockParamUnion
			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					content = append(content, anthropic.NewTextBlock(textPart.Text))
				}
			}
			if len(content) > 0 {
				apiMessages = append(apiMessages, anthropic.NewUserMessage(content...))
			}

		case message.TypeAI:
			aiMsg, ok := msg.(*message.AIMessage)
			if !ok {
				continue
			}

			var content []anthropic.ContentBlockParamUnion

			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					content = append(content, anthropic.NewTextBlock(textPart.Text))
				}
			}

			for _, tc := range aiMsg.ToolCalls {
				inputJSON, _ := json.Marshal(tc.Arguments)
				content = append(content, anthropic.NewToolUseBlock(tc.ID, tc.Name, string(inputJSON)))
			}

			if len(content) > 0 {
				apiMessages = append(apiMessages, anthropic.NewAssistantMessage(content...))
			}

		case message.TypeTool:
			toolMsg, ok := msg.(*message.ToolMessage)
			if !ok {
				continue
			}

			var resultStr string
			parts := toolMsg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					resultStr = textPart.Text
				}
			}
			if resultStr == "" {
				resultStr = fmt.Sprintf("%v", parts)
			}

			apiMessages = append(apiMessages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(toolMsg.ToolCallID, resultStr, false),
			))
		}
	}

	return apiMessages, systemPrompt, nil
}

func convertToolsToAnthropic(tools []tool.Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()

		schemaJSON, err := json.Marshal(def.Function.Parameters)
		if err != nil {
			continue
		}

		var schemaMap map[string]any
		if err := json.Unmarshal(schemaJSON, &schemaMap); err != nil {
			continue
		}

		toolParam := anthropic.ToolParam{
			Name:        def.Function.Name,
			Description: param.NewOpt(def.Function.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: schemaMap,
			},
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &toolParam})
	}

	return result
}

func convertAnthropicResponseToMessage(resp *anthropic.Message) (message.Message, error) {
	var textParts []message.Part
	var toolCalls []message.ToolCall

	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			textParts = append(textParts, message.TextPart{Text: b.Text})

		case anthropic.ToolUseBlock:
			var inputMap map[string]any
			if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &inputMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool input: %w", err)
			}

			toolCalls = append(toolCalls, message.ToolCall{
				ID:        b.ID,
				Name:      b.Name,
				Type:      "function",
				Arguments: inputMap,
			})
		}
	}

	aiMsg := message.NewAIMessage(textParts)
	if len(toolCalls) > 0 {
		aiMsg.ToolCalls = toolCalls
	}

	if len(textParts) == 0 && len(toolCalls) == 0 {
		return nil, fmt.Errorf("anthropic response contained no content")
	}

	return aiMsg, nil
}

func newStreamFromAnthropic(stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) *model.Stream {
	chunks := make(chan model.StreamChunk)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(chunks)
		defer func() { _ = stream.Close() }() // Best effort close

		var textBuffer strings.Builder

		for stream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			event := stream.Current()

			if deltaEvent, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if delta, ok := deltaEvent.Delta.AsAny().(anthropic.TextDelta); ok {
					textBuffer.WriteString(delta.Text)
					chunks <- model.StreamChunk{
						Text: delta.Text,
					}
				}
			} else if _, ok := event.AsAny().(anthropic.MessageStopEvent); ok {
				if textBuffer.Len() > 0 {
					chunks <- model.StreamChunk{
						Text:  textBuffer.String(),
						Final: true,
					}
				}
				return
			}
		}

		if err := stream.Err(); err != nil {
			chunks <- model.StreamChunk{
				Err:   err,
				Final: true,
			}
		}
	}()

	return model.NewStream(chunks, cancel)
}

// Compile-time interface checks
var (
	_ model.Model     = (*Model)(nil)
	_ model.ToolAware = (*Model)(nil)
)
