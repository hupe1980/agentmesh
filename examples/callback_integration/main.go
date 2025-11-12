package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// This example demonstrates plugin integration with AgentMesh.
// It shows how to use plugins for:
// - Request validation (BeforeModel)
// - Response transformation (AfterModel)
// - Tool access control (BeforeTool)
// - Tool result transformation (AfterTool)
// - Error handling (OnToolError, OnModelError)

func main() {
	fmt.Println("=== AgentMesh Plugin Integration Demo ===")
	fmt.Println()

	// Create plugin manager
	pluginMgr := callbacks.NewPluginManager()

	// Create and register custom plugin
	customPlugin := &DemoPlugin{}
	if err := pluginMgr.Register(context.Background(), customPlugin); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Plugin manager configured with DemoPlugin")
	fmt.Println("  - BeforeModel: validates requests")
	fmt.Println("  - AfterModel: sanitizes responses")
	fmt.Println("  - BeforeTool: validates tool access")
	fmt.Println("  - AfterTool: transforms results")
	fmt.Println("  - OnModelError: handles model failures")
	fmt.Println("  - OnToolError: handles tool failures")
	fmt.Println()

	// Note: In a real application, you would pass the plugin manager to agents:
	//
	// agent := agent.NewReActAgent(
	//     myModel,
	//     tools,
	//     agent.WithPluginManager(pluginMgr),
	// )

	fmt.Println("Agent Integration:")
	fmt.Println("  agent := agent.NewReActAgent(model, tools, agent.WithPluginManager(pluginMgr))")
	fmt.Println()

	fmt.Println("Execution Flow:")
	fmt.Println("  1. BeforeModel validates request")
	fmt.Println("  2. Model generates response (if not short-circuited)")
	fmt.Println("  3. AfterModel sanitizes response")
	fmt.Println("  4. BeforeTool validates tool access")
	fmt.Println("  5. Tool executes (if not blocked)")
	fmt.Println("  6. AfterTool transforms result")
	fmt.Println("  7. OnToolError handles failures (if tool failed)")
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}

// DemoPlugin demonstrates a custom plugin implementation
type DemoPlugin struct {
	callbacks.NoopPlugin
}

// BeforeModel validates requests before calling the model
func (p *DemoPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("empty message list")
	}

	// Example: block requests with long content
	lastMsg := req.Messages[len(req.Messages)-1]
	text := message.Stringify(lastMsg)

	if len(text) > 10000 {
		return nil, fmt.Errorf("request too long: %d characters", len(text))
	}

	return nil, nil // Continue to model
}

// AfterModel sanitizes model responses
func (p *DemoPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	// Example: redact sensitive information
	text := message.Stringify(resp.Message)

	if containsSensitiveData(text) {
		// Return sanitized version
		return &model.Response{
			Message: message.NewAIMessageFromText("[Response sanitized for safety]"),
		}, nil
	}

	return nil, nil // Keep original
}

// BeforeTool validates tool access
func (p *DemoPlugin) BeforeTool(ctx context.Context, toolName string, input any) error {
	// Example: check if user has permission to use this tool
	restrictedTools := []string{"delete_database", "execute_command"}

	for _, restricted := range restrictedTools {
		if toolName == restricted {
			return fmt.Errorf("access denied: tool %s requires elevated permissions", toolName)
		}
	}

	return nil // Continue to tool execution
}

// AfterTool transforms tool results
func (p *DemoPlugin) AfterTool(ctx context.Context, toolName string, result callbacks.ToolResult) error {
	// Log tool execution
	log.Printf("Tool %s executed in %v", toolName, result.Duration)
	return nil
}

// OnToolError handles tool execution failures
func (p *DemoPlugin) OnToolError(ctx context.Context, toolName string, err error) error {
	// Example: log error for monitoring
	log.Printf("Tool %s failed: %v", toolName, err)

	// Propagate error (no fallback in this example)
	return nil
}

// OnModelError handles model failures
func (p *DemoPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	// Example: provide fallback for certain errors
	log.Printf("Model call failed: %v", err)

	if isTransientError(err) {
		// Provide fallback response
		return &model.Response{
			Message: message.NewAIMessageFromText("Model temporarily unavailable. Please try again."),
		}, nil
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
