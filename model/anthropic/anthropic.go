// Package anthropic provides a model wrapper for the Anthropic Claude API.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
)

// Options configures the Anthropic model adapter (temperature, model id,
// max tokens, API key). Extend via functional options to preserve stability.
type Options struct {
	Model       anthropic.Model
	Temperature float64
	MaxTokens   int64
	APIKey      string
}

// Model wraps the Anthropic Messages API behind the generic model.Model interface.
type Model struct {
	client *anthropic.Client
	opts   Options
}

// NewModel creates a new Anthropic model using the official client
func NewModel(optFns ...func(o *Options)) *Model {
	opts := Options{
		Model:       anthropic.ModelClaude3_5Sonnet20241022,
		Temperature: 0.7,
		MaxTokens:   4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	var clientOpts []option.RequestOption
	if opts.APIKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.APIKey))
	}

	client := anthropic.NewClient(clientOpts...)

	return &Model{
		client: &client,
		opts:   opts,
	}
}

// NewModelFromClient creates a new Anthropic model from an existing client
func NewModelFromClient(client *anthropic.Client, optFns ...func(o *Options)) *Model {
	opts := Options{
		Model:       anthropic.ModelClaude3_5Sonnet20241022,
		Temperature: 0.7,
		MaxTokens:   4096,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Model{
		client: client,
		opts:   opts,
	}
}

// Generate implements unified streaming / non-streaming generation.
// It adapts Anthropic Messages API (with function/tool calling) into model.Response events.
func (m *Model) Generate(ctx context.Context, req model.Request) (<-chan model.Response, <-chan error) {
	out := make(chan model.Response, 32)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		// Build messages for Anthropic API
		messages := m.buildMessages(req.Contents)

		// Build the message request
		params := anthropic.MessageNewParams{
			Model:       m.opts.Model,
			Messages:    messages,
			MaxTokens:   m.opts.MaxTokens,
			Temperature: anthropic.Float(m.opts.Temperature),
		}

		// Add system message if present
		if systemBlocks := m.extractSystemMessage(req.Contents); len(systemBlocks) > 0 {
			params.System = systemBlocks
		}

		// Add tools if present
		if len(req.Tools) > 0 {
			tools := m.buildTools(req.Tools)
			params.Tools = tools
		}

		if req.Stream {
			// TODO: Implement streaming support
			// Streaming implementation would require handling:
			// - anthropic.MessageStreamEvent types
			// - Partial text accumulation
			// - Tool use detection and response handling
			// - Proper event-based content building
			errCh <- fmt.Errorf("streaming not yet implemented for Anthropic model")
			return
		}

		// Non-streaming path
		resp, err := m.client.Messages.New(ctx, params)
		if err != nil {
			errCh <- fmt.Errorf("anthropic api error: %w", err)
			return
		}

		// Build content parts (text + function calls)
		var parts []core.Part

		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				textBlock := block.AsText()
				if textBlock.Text != "" {
					parts = append(parts, core.TextPart{Text: textBlock.Text})
				}
			case "tool_use":
				toolBlock := block.AsToolUse()
				args := ""
				if toolBlock.Input != nil {
					if argsBytes, err := json.Marshal(toolBlock.Input); err == nil {
						args = string(argsBytes)
					}
				}
				parts = append(parts, core.FunctionCallPart{
					FunctionCall: core.FunctionCall{
						ID:        toolBlock.ID,
						Name:      toolBlock.Name,
						Arguments: args,
					},
				})
			}
		}

		finishReason := "stop"
		if resp.StopReason != "" {
			finishReason = string(resp.StopReason)
		}

		out <- model.Response{
			Partial:      false,
			Content:      core.Content{Role: "assistant", Parts: parts},
			FinishReason: finishReason,
		}
	}()

	return out, errCh
}

// buildMessages converts AgentMesh contents to Anthropic message format.
func (m *Model) buildMessages(contents []core.Content) []anthropic.MessageParam {
	var messages []anthropic.MessageParam

	// Track tool responses for proper ordering
	toolResponses := make(map[string]string)
	for _, c := range contents {
		if c.Role == "tool" {
			for _, p := range c.Parts {
				if fr, ok := p.(core.FunctionResponsePart); ok {
					if fr.FunctionResponse.ID != "" {
						if respStr, ok := fr.FunctionResponse.Response.(string); ok {
							toolResponses[fr.FunctionResponse.ID] = respStr
						} else {
							toolResponses[fr.FunctionResponse.ID] = fmt.Sprintf("%v", fr.FunctionResponse.Response)
						}
					}
				}
			}
		}
	}

	for _, c := range contents {
		if c.Role == "system" || c.Role == "tool" {
			continue // System messages handled separately, tool responses embedded
		}

		switch c.Role {
		case "user":
			content := m.buildUserContent(c.Parts)
			if len(content) > 0 {
				messages = append(messages, anthropic.NewUserMessage(content...))
			}
		case "assistant":
			content := m.buildAssistantContent(c.Parts, toolResponses)
			if len(content) > 0 {
				messages = append(messages, anthropic.NewAssistantMessage(content...))
			}
		default:
			// Treat unknown roles as user
			content := m.buildUserContent(c.Parts)
			if len(content) > 0 {
				messages = append(messages, anthropic.NewUserMessage(content...))
			}
		}
	}

	return messages
}

// extractSystemMessage extracts system message blocks
func (m *Model) extractSystemMessage(contents []core.Content) []anthropic.TextBlockParam {
	var systemBlocks []anthropic.TextBlockParam

	for _, c := range contents {
		if c.Role == "system" {
			for _, p := range c.Parts {
				if tp, ok := p.(core.TextPart); ok && tp.Text != "" {
					systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
						Text: tp.Text,
					})
				}
			}
		}
	}

	return systemBlocks
}

// buildUserContent builds content for user messages
func (m *Model) buildUserContent(parts []core.Part) []anthropic.ContentBlockParamUnion {
	var content []anthropic.ContentBlockParamUnion

	for _, p := range parts {
		if tp, ok := p.(core.TextPart); ok && tp.Text != "" {
			content = append(content, anthropic.NewTextBlock(tp.Text))
		}
	}

	return content
}

// buildAssistantContent builds content for assistant messages
func (m *Model) buildAssistantContent(
	parts []core.Part,
	toolResponses map[string]string,
) []anthropic.ContentBlockParamUnion {
	var content []anthropic.ContentBlockParamUnion
	var toolCallIDs []string

	for _, p := range parts {
		switch part := p.(type) {
		case core.TextPart:
			if part.Text != "" {
				content = append(content, anthropic.NewTextBlock(part.Text))
			}
		case core.FunctionCallPart:
			// Parse the arguments JSON for the tool call
			var input interface{}
			if part.FunctionCall.Arguments != "" {
				if err := json.Unmarshal([]byte(part.FunctionCall.Arguments), &input); err != nil {
					input = part.FunctionCall.Arguments // fallback to string
				}
			}

			content = append(content, anthropic.NewToolUseBlock(
				part.FunctionCall.ID,
				input,
				part.FunctionCall.Name,
			))
			toolCallIDs = append(toolCallIDs, part.FunctionCall.ID)
		}
	}

	// Add tool responses immediately after tool calls
	for _, id := range toolCallIDs {
		if resp, ok := toolResponses[id]; ok {
			content = append(content, anthropic.NewToolResultBlock(id, resp, false))
			delete(toolResponses, id)
		}
	}

	return content
}

// buildTools converts agentmesh tools to Anthropic tool format
func (m *Model) buildTools(tools []model.ToolDefinition) []anthropic.ToolUnionParam {
	anthropicTools := make([]anthropic.ToolUnionParam, len(tools))

	for i, tool := range tools {
		// Build input schema from function parameters
		inputSchema := anthropic.ToolInputSchemaParam{
			Type: constant.Object("object"), // Default to object type
		}

		// Copy the schema properties
		if tool.Function.Parameters != nil {
			params := tool.Function.Parameters
			if properties, exists := params["properties"]; exists {
				inputSchema.Properties = properties
			}
			if required, exists := params["required"]; exists {
				if reqSlice, ok := required.([]string); ok {
					inputSchema.Required = reqSlice
				} else if reqInterface, ok := required.([]interface{}); ok {
					// Convert []interface{} to []string
					var reqStrings []string
					for _, r := range reqInterface {
						if s, ok := r.(string); ok {
							reqStrings = append(reqStrings, s)
						}
					}
					inputSchema.Required = reqStrings
				}
			}
		}

		anthropicTools[i] = anthropic.ToolUnionParamOfTool(inputSchema, tool.Function.Name)
	}

	return anthropicTools
}

// Info returns metadata describing this Anthropic model implementation.
func (m *Model) Info() model.Info {
	return model.Info{
		Name:          string(m.opts.Model),
		Provider:      "anthropic",
		SupportsTools: true,
	}
}
