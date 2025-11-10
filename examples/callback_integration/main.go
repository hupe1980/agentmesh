package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// This example demonstrates callback integration with ModelNode and ToolNode.
// It shows how to use callbacks for:
// - Request validation (BeforeModel)
// - Response transformation (AfterModel)
// - Tool access control (BeforeTool)
// - Tool result transformation (AfterTool)
// - Error handling (OnToolError)

func main() {
	fmt.Println("=== AgentMesh Callback Integration Demo ===")
	fmt.Println()

	// Create callback manager
	cbManager := callbacks.NewManager()

	// Register model callbacks
	cbManager.RegisterBeforeModel(validateRequest)
	cbManager.RegisterAfterModel(sanitizeResponse)

	// Register tool callbacks
	cbManager.RegisterBeforeTool(validateToolAccess)
	cbManager.RegisterAfterTool(transformToolResult)
	cbManager.RegisterOnToolError(handleToolError)

	fmt.Println("✓ Callback manager configured with 5 callbacks")
	fmt.Println("  - BeforeModel: validateRequest")
	fmt.Println("  - AfterModel: sanitizeResponse")
	fmt.Println("  - BeforeTool: validateToolAccess")
	fmt.Println("  - AfterTool: transformToolResult")
	fmt.Println("  - OnToolError: handleToolError")
	fmt.Println()

	// Note: In a real application, you would create ModelNode and ToolNode like this:
	//
	// modelNode := agent.ModelNode(
	//     myModel,
	//     agent.WithModelCallbacks(cbManager),
	// )
	//
	// toolNode := agent.ToolNode(
	//     toolRegistry,
	//     agent.WithToolCallbacks(cbManager),
	// )

	fmt.Println("ModelNode Integration:")
	fmt.Println("  modelNode := agent.ModelNode(myModel, agent.WithModelCallbacks(cbManager))")
	fmt.Println()

	fmt.Println("ToolNode Integration:")
	fmt.Println("  toolNode := agent.ToolNode(toolRegistry, agent.WithToolCallbacks(cbManager))")
	fmt.Println()

	fmt.Println("Execution Flow:")
	fmt.Println("  1. BeforeModel validates request")
	fmt.Println("  2. Model generates response (if not short-circuited)")
	fmt.Println("  3. AfterModel sanitizes response")
	fmt.Println("  4. BeforeTool validates tool access")
	fmt.Println("  5. Tool executes (if not short-circuited)")
	fmt.Println("  6. AfterTool transforms result")
	fmt.Println("  7. OnToolError handles failures (if tool failed)")
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}

// validateRequest is a BeforeModel callback that validates requests before calling the model
func validateRequest(ctx context.Context, s graph.StateWriter) (message.Message, error) {
	events := s.MessageEventsSnapshot()
	if len(events) == 0 {
		return nil, fmt.Errorf("empty message list")
	}

	// Example: block requests with certain keywords
	lastMsg := events[len(events)-1].Message
	parts := lastMsg.Parts()
	for _, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			if len(textPart.Text) > 10000 {
				return nil, fmt.Errorf("request too long: %d characters", len(textPart.Text))
			}
		}
	}

	return nil, nil // Continue to model
}

// sanitizeResponse is an AfterModel callback that sanitizes model responses
func sanitizeResponse(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
	// Example: redact sensitive information
	// In production, use proper PII detection

	aiMsg, ok := response.(*message.AIMessage)
	if !ok {
		return nil, nil // Keep original
	}

	// Check if response contains sensitive patterns
	parts := aiMsg.Parts()
	needsSanitization := false

	for _, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			if len(textPart.Text) > 0 {
				// Simple check - in production use proper detection
				if containsSensitiveData(textPart.Text) {
					needsSanitization = true
					break
				}
			}
		}
	}

	if needsSanitization {
		// Return sanitized version
		return message.NewAIMessageFromText("[Response sanitized for safety]"), nil
	}

	return nil, nil // Keep original
}

// validateToolAccess is a BeforeTool callback that validates tool access
func validateToolAccess(ctx context.Context, s graph.StateWriter, call message.ToolCall) (any, error) {
	// Example: check if user has permission to use this tool
	// In production, check actual user permissions from context

	restrictedTools := []string{"delete_database", "execute_command"}
	for _, restricted := range restrictedTools {
		if call.Name == restricted {
			return nil, fmt.Errorf("access denied: tool %s requires elevated permissions", call.Name)
		}
	}

	return nil, nil // Continue to tool execution
}

// transformToolResult is an AfterTool callback that transforms tool results
func transformToolResult(ctx context.Context, s graph.StateWriter, call message.ToolCall, result any) (any, error) {
	// Example: format results consistently
	// In production, apply proper transformation logic

	if result == nil {
		return "No result", nil
	}

	// Wrap result in standard format
	return map[string]any{
		"tool":      call.Name,
		"result":    result,
		"processed": true,
	}, nil
}

// handleToolError is an OnToolError callback that handles tool execution failures
func handleToolError(ctx context.Context, s graph.StateWriter, call message.ToolCall, err error) (any, error) {
	// Example: provide fallback for certain errors
	// In production, implement proper error handling strategy

	log.Printf("Tool %s failed: %v", call.Name, err)

	// Provide fallback for known transient errors
	if isTransientError(err) {
		return fmt.Sprintf("Tool temporarily unavailable: %s", call.Name), nil
	}

	// Propagate error for unknown failures
	return nil, err
}

// Helper functions

func containsSensitiveData(text string) bool {
	// Simplified check - in production use proper PII detection
	return false
}

func isTransientError(err error) bool {
	// Simplified check - in production check for specific error types
	return false
}
