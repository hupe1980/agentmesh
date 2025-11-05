/*
Package message provides types for representing multi-modal conversational messages
between humans, AI assistants, tools, and systems.

# Overview

Messages are the fundamental unit of communication in agent conversations. The
message package defines structured message types with support for:
  - Text content
  - Structured data
  - File attachments
  - Tool calls and responses
  - Multiple content parts per message

# Message Types

Different message types represent different conversation participants:

	// System message (instructions for AI)
	system := message.NewSystemMessageFromText("You are a helpful assistant")

	// Human message (user input)
	human := message.NewHumanMessageFromText("Hello, how are you?")

	// AI message (assistant response)
	ai := message.NewAIMessageFromText("I'm doing well, thank you!")

	// Tool message (tool execution result)
	tool := message.NewToolMessage(toolID, toolName, resultJSON)

# Multi-Part Messages

Messages can contain multiple content parts:

	parts := message.Parts{
		message.TextPart{Text: "Here is the image:"},
		message.FilePart{
			File: message.FileURI{URI: "https://example.com/image.jpg"},
			MimeType: "image/jpeg",
		},
	}
	msg := message.NewAIMessage(parts)

# Content Types

Supported content part types:

  - TextPart: Plain UTF-8 text
  - DataPart: Structured data (JSON objects)
  - FilePart: File attachments (bytes, base64, path, or URI)
  - FunctionCallPart: Tool invocation requests
  - FunctionResponsePart: Tool execution results

# Tool Calling

AI messages can request tool execution:

	calls := []message.FunctionCall{
		{
			ID:        "call_123",
			Name:      "get_weather",
			Arguments: `{"location": "Boston"}`,
		},
	}
	aiMsg := message.NewAIMessage(nil)
	aiMsg.ToolCalls = calls

Tool results are returned as tool messages:

	result := message.NewToolMessage(
		"call_123",
		"get_weather",
		`{"temperature": 72, "conditions": "sunny"}`,
	)

# Message Interface

All message types implement the Message interface:

	type Message interface {
		Type() Type
		Parts() Parts
		Clone() Message
	}

# Cloning

Messages are immutable - use Clone() to create copies:

	original := message.NewHumanMessageFromText("Hello")
	cloned := original.Clone()

# Conversation History

Messages are typically stored in slices:

	conversation := []message.Message{
		message.NewSystemMessageFromText("You are helpful"),
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there!"),
	}

# Type Constants

Message types are identified by constants:

	const (
		TypeSystem   Type = "system"   // System instructions
		TypeHuman    Type = "human"    // User input
		TypeAI       Type = "ai"       // Assistant response
		TypeChat     Type = "chat"     // Generic chat message
		TypeFunction Type = "function" // Function/tool call
		TypeTool     Type = "tool"     // Tool result
	)

# Best Practices

  - Use appropriate message types for each participant
  - Keep system messages concise and clear
  - Structure tool responses as JSON when possible
  - Clone messages before mutation
  - Preserve message order in conversations
*/
package message
