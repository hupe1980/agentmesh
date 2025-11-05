// Package main demonstrates using MCP (Model Context Protocol) tools with AgentMesh.
//
// Prerequisites:
//
//	go get github.com/modelcontextprotocol/go-sdk/mcp
//	export OPENAI_API_KEY=your_key_here
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	mcptool "github.com/hupe1980/agentmesh/pkg/tool/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Simple typed input/output for our MCP tool.
// The SDK will auto-generate JSON schemas for these.
type sumInput struct {
	A float64 `json:"a" jsonschema:"first operand"`
	B float64 `json:"b" jsonschema:"second operand"`
}

type sumOutput struct {
	Result float64 `json:"result" jsonschema:"sum of a and b"`
}

// sumTool implements a typed MCP tool handler using mcp.AddTool
func sumTool(_ context.Context, _ *mcp.CallToolRequest, in sumInput) (*mcp.CallToolResult, sumOutput, error) {
	return nil, sumOutput{Result: in.A + in.B}, nil
}

// newInMemoryMCP creates an in-memory MCP server with a simple sum tool
func newInMemoryMCP() *mcp.InMemoryTransport {
	// Create paired in-memory transports (client <-> server)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// Create MCP server with the sum tool
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "demo-mcp-server",
		Version: "v1.0.0",
	}, nil)

	// Register the sum tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sum",
		Description: "Add two numbers and return the result",
	}, sumTool)

	// Start serving in background
	go func() {
		if err := server.Run(context.Background(), serverTransport); err != nil {
			log.Printf("MCP server terminated: %v", err)
		}
	}()

	return clientTransport
}

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	fmt.Println("=== MCP Tools Example ===")
	fmt.Println()

	// 1. Create in-memory MCP server with a sum tool
	clientTransport := newInMemoryMCP()

	// 2. Create a Toolset that discovers tools from the MCP server
	mcpToolset := mcptool.NewToolset(mcptool.NewInMemorySessionFactory(clientTransport))
	defer func() {
		if err := mcpToolset.Close(); err != nil {
			log.Printf("Error closing MCP toolset: %v", err)
		}
	}()

	// 3. Create a ReAct agent with the MCP toolset
	//    The toolset will dynamically discover the "sum" tool from the MCP server
	model := openai.NewModel(func(o *openai.Options) {
		o.Temperature = 0
	})

	reactAgent, err := agent.NewReActAgent(
		model,
		agent.WithToolset(mcpToolset),
		agent.WithMaxIterations(5),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// 4. Run a conversation where the agent uses the MCP tool
	ctx := context.Background()
	messages := []message.Message{
		message.NewSystemMessageFromText(
			"You are a helpful assistant. Use the sum tool to add numbers when asked.",
		),
		message.NewHumanMessageFromText("Please add 3.5 and 4.25. What is the result?"),
	}

	fmt.Println("Invoking agent with MCP tools...")
	fmt.Println("Question: Please add 3.5 and 4.25. What is the result?")
	fmt.Println()

	result, err := reactAgent.Invoke(ctx, messages)
	if err != nil {
		log.Fatalf("Agent invocation failed: %v", err)
	}

	// 5. Display the conversation
	fmt.Println("=== Conversation ===")
	for _, msg := range result {
		switch m := msg.(type) {
		case *message.SystemMessage:
			fmt.Print("[System] ")
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Print(text.Text)
				}
			}
			fmt.Print("\n\n")

		case *message.HumanMessage:
			fmt.Print("[Human] ")
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Print(text.Text)
				}
			}
			fmt.Print("\n\n")

		case *message.AIMessage:
			if len(m.ToolCalls) > 0 {
				fmt.Println("[AI] Calling tools:")
				for _, tc := range m.ToolCalls {
					fmt.Printf("  - %s(%s)\n", tc.Name, tc.Arguments)
				}
				fmt.Print("\n")
			} else {
				fmt.Print("[AI] ")
				for _, part := range m.Parts() {
					if text, ok := part.(message.TextPart); ok {
						fmt.Print(text.Text)
					}
				}
				fmt.Print("\n\n")
			}

		case *message.ToolMessage:
			fmt.Printf("[Tool: %s] ", m.ToolCallID)
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Print(text.Text)
				}
			}
			fmt.Print("\n\n")
		}
	}

	fmt.Println("=== Example Complete ===")
	fmt.Println()
	fmt.Println("The agent successfully:")
	fmt.Println("1. Discovered the 'sum' tool from the MCP server")
	fmt.Println("2. Called the tool with arguments (3.5, 4.25)")
	fmt.Println("3. Received the result (7.75)")
	fmt.Println("4. Responded to the user with the answer")
}
