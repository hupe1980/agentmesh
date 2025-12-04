package amazonbedrock

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// convertMessagesToBedrock converts AgentMesh messages to Bedrock Converse format.
// Returns the converted messages and any extracted system prompt.
func convertMessagesToBedrock(msgs []message.Message) ([]types.Message, string) {
	var bedrockMsgs []types.Message
	var systemPrompt string

	for _, msg := range msgs {
		switch msg.Type() {
		case message.TypeSystem:
			// Extract system message text
			systemPrompt = message.Stringify(msg)

		case message.TypeHuman:
			content := convertPartsToBedrock(msg.Parts())
			if len(content) > 0 {
				bedrockMsgs = append(bedrockMsgs, types.Message{
					Role:    types.ConversationRoleUser,
					Content: content,
				})
			}

		case message.TypeAI:
			content := convertPartsToBedrock(msg.Parts())
			// Add tool use blocks if present
			if aiMsg, ok := msg.(*message.AIMessage); ok {
				for _, tc := range aiMsg.ToolCalls {
					content = append(content, convertToolCallToBedrock(tc))
				}
			}
			if len(content) > 0 {
				bedrockMsgs = append(bedrockMsgs, types.Message{
					Role:    types.ConversationRoleAssistant,
					Content: content,
				})
			}

		case message.TypeTool:
			// Tool results go as user messages with tool result blocks
			if toolMsg, ok := msg.(*message.ToolMessage); ok {
				content := convertToolResultToBedrock(toolMsg)
				if len(content) > 0 {
					bedrockMsgs = append(bedrockMsgs, types.Message{
						Role:    types.ConversationRoleUser,
						Content: content,
					})
				}
			}
		}
	}

	return bedrockMsgs, systemPrompt
}

// convertPartsToBedrock converts message parts to Bedrock content blocks.
func convertPartsToBedrock(parts message.Parts) []types.ContentBlock {
	var blocks []types.ContentBlock

	for _, part := range parts {
		switch p := part.(type) {
		case message.TextPart:
			blocks = append(blocks, &types.ContentBlockMemberText{
				Value: p.Text,
			})
		case *message.TextPart:
			if p != nil {
				blocks = append(blocks, &types.ContentBlockMemberText{
					Value: p.Text,
				})
			}
		case message.FilePart:
			if block := convertFilePartToBedrock(&p); block != nil {
				blocks = append(blocks, block)
			}
		case *message.FilePart:
			if block := convertFilePartToBedrock(p); block != nil {
				blocks = append(blocks, block)
			}
		}
	}

	return blocks
}

// convertFilePartToBedrock converts a file part to a Bedrock content block.
// Supports image files via base64 encoding.
func convertFilePartToBedrock(fp *message.FilePart) types.ContentBlock {
	if fp == nil || !strings.HasPrefix(fp.MimeType, "image/") {
		return nil
	}

	data := extractImageData(fp)
	if len(data) == 0 {
		return nil
	}

	return &types.ContentBlockMemberImage{
		Value: types.ImageBlock{
			Format: mimeToImageFormat(fp.MimeType),
			Source: &types.ImageSourceMemberBytes{
				Value: data,
			},
		},
	}
}

// extractImageData extracts raw bytes from a FilePart.
func extractImageData(fp *message.FilePart) []byte {
	switch fc := fp.File.(type) {
	case message.FileRawBytes:
		return fc.Bytes
	case *message.FileRawBytes:
		if fc != nil {
			return fc.Bytes
		}
	case message.FileBase64:
		decoded, err := base64.StdEncoding.DecodeString(fc.Base64)
		if err != nil {
			return nil
		}
		return decoded
	case *message.FileBase64:
		if fc != nil {
			decoded, err := base64.StdEncoding.DecodeString(fc.Base64)
			if err != nil {
				return nil
			}
			return decoded
		}
	}
	return nil
}

// mimeToImageFormat converts MIME type to Bedrock ImageFormat.
func mimeToImageFormat(mimeType string) types.ImageFormat {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return types.ImageFormatJpeg
	case "image/png":
		return types.ImageFormatPng
	case "image/gif":
		return types.ImageFormatGif
	case "image/webp":
		return types.ImageFormatWebp
	default:
		return types.ImageFormatJpeg
	}
}

// convertToolCallToBedrock converts a tool call to a Bedrock tool use block.
func convertToolCallToBedrock(tc message.ToolCall) types.ContentBlock {
	// ToolCall.Arguments is a JSON string - parse it for document interface
	var args map[string]any
	if tc.Arguments != "" {
		_ = json.Unmarshal([]byte(tc.Arguments), &args)
	}
	if args == nil {
		args = make(map[string]any)
	}

	return &types.ContentBlockMemberToolUse{
		Value: types.ToolUseBlock{
			ToolUseId: aws.String(tc.ID),
			Name:      aws.String(tc.Name),
			Input:     document.NewLazyDocument(args),
		},
	}
}

// convertToolResultToBedrock converts a tool message to Bedrock tool result blocks.
func convertToolResultToBedrock(toolMsg *message.ToolMessage) []types.ContentBlock {
	// Get the text content from the tool message
	var resultStr string
	parts := toolMsg.Parts()
	if len(parts) > 0 {
		if textPart, ok := parts[0].(message.TextPart); ok {
			resultStr = textPart.Text
		}
	}

	return []types.ContentBlock{
		&types.ContentBlockMemberToolResult{
			Value: types.ToolResultBlock{
				ToolUseId: aws.String(toolMsg.ToolCallID),
				Content: []types.ToolResultContentBlock{
					&types.ToolResultContentBlockMemberText{
						Value: resultStr,
					},
				},
			},
		},
	}
}

// convertToolsToBedrock converts AgentMesh tools to Bedrock tool configuration.
func convertToolsToBedrock(tools []tool.Tool) *types.ToolConfiguration {
	bedrockTools := make([]types.Tool, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()
		if def == nil {
			continue
		}

		// Convert Parameters map to document.Interface
		schema := def.Function.Parameters
		if schema == nil {
			schema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}

		bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(t.Name()),
				Description: aws.String(t.Description()),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(schema),
				},
			},
		})
	}

	return &types.ToolConfiguration{
		Tools: bedrockTools,
	}
}

// convertBedrockOutputToMessage converts Bedrock output to an AgentMesh message.
func convertBedrockOutputToMessage(output *bedrockruntime.ConverseOutput) message.Message {
	if output.Output == nil {
		return nil
	}

	msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return nil
	}

	var textParts []string
	var toolCalls []message.ToolCall

	for _, block := range msgOutput.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			textParts = append(textParts, b.Value)
		case *types.ContentBlockMemberToolUse:
			// Unmarshal document.Interface to get the arguments
			var args map[string]any
			if b.Value.Input != nil {
				_ = b.Value.Input.UnmarshalSmithyDocument(&args)
			}
			// Convert args map to JSON string for ToolCall.Arguments
			argsJSON, _ := json.Marshal(args)
			toolCalls = append(toolCalls, message.ToolCall{
				ID:        aws.ToString(b.Value.ToolUseId),
				Name:      aws.ToString(b.Value.Name),
				Type:      "function",
				Arguments: string(argsJSON),
			})
		}
	}

	// Build the AI message
	if len(toolCalls) > 0 {
		aiMsg := message.NewAIMessage(message.Parts{
			message.TextPart{Text: strings.Join(textParts, "")},
		})
		aiMsg.ToolCalls = toolCalls
		return aiMsg
	}

	return message.NewAIMessageFromText(strings.Join(textParts, ""))
}

// convertUsage converts Bedrock token usage to AgentMesh format.
func convertUsage(usage *types.TokenUsage) *model.UsageInfo {
	if usage == nil {
		return nil
	}
	return &model.UsageInfo{
		PromptTokens:     int(aws.ToInt32(usage.InputTokens)),
		CompletionTokens: int(aws.ToInt32(usage.OutputTokens)),
		TotalTokens:      int(aws.ToInt32(usage.TotalTokens)),
	}
}
