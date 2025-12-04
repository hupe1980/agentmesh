// Package main demonstrates Docker-based tool execution with an AI agent.
// This example shows:
//   - Creating Docker tools that run commands in isolated containers
//   - Using network tools (curl) in a ReAct agent
//   - Processing containerized command output
//
// Prerequisites:
//   - Docker daemon running
//   - OPENAI_API_KEY environment variable set
//
// Run: OPENAI_API_KEY=sk-... go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/tool/docker"
)

func main() {
	// Validate API key is set
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	// Create a Docker-based curl tool
	// This tool runs curl commands inside an isolated container with:
	//   - Network access (bridge mode) to make HTTP requests
	//   - 30 second timeout
	//   - 256MB memory limit
	//   - 0.5 CPU quota
	curlTool, err := docker.NewTool("curl_request", "curlimages/curl:latest",
		docker.WithDescription("Make HTTP requests using curl. Pass curl arguments as the command. Example: '-s -I https://example.com' to get headers."),
		docker.WithTimeout(30*time.Second),
		docker.WithNetworkMode("bridge"), // Needs network access for HTTP
		docker.WithPullImage(true),       // Pull image if not present
	)
	if err != nil {
		log.Fatalf("failed to create curl tool: %v", err)
	}
	defer curlTool.Close()

	// Build a ReAct agent with the Docker tool
	compiled, err := agent.NewReActAgent(
		openai.NewModel(),
		agent.WithTools(curlTool),
		agent.WithMaxIterations(3),
	)
	if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

	// Prepare conversation
	system := message.NewSystemMessageFromText(
		`You are a helpful assistant with access to curl for making HTTP requests.
When asked about websites or APIs, use the curl_request tool to fetch information.
The command should be curl arguments only (the curl binary is the entrypoint).
Examples:
  - Get headers: "-s -I https://example.com"
  - Get content: "-s https://api.example.com/data"
  - POST request: "-s -X POST -d '{"key":"value"}' https://api.example.com"`,
	)
	human := message.NewHumanMessageFromText("What HTTP headers does https://httpbin.org/get return?")

	// Execute the agent
	fmt.Println("=== Docker Tool Agent ===")
	fmt.Println()

	i := 0
	for evt, err := range compiled.Run(ctx, []message.Message{system, human}) {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}

		if evt == nil {
			continue
		}

		fmt.Printf("[%d] %s\n", i+1, evt.Type())

		switch m := evt.(type) {
		case *message.AIMessage:
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok && text.Text != "" {
					fmt.Printf("    💭 %s\n", text.Text)
				}
			}
			if message.HasToolCalls(evt) {
				for _, tc := range m.ToolCalls {
					fmt.Printf("    🐳 Docker tool call: %s(%s)\n", tc.Name, tc.Arguments)
				}
			}

		case *message.ToolMessage:
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					// Truncate long output for display
					output := text.Text
					if len(output) > 500 {
						output = output[:500] + "..."
					}
					fmt.Printf("    ⚙️  Container output:\n%s\n", indent(output, "       "))
				}
			}

		default:
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Printf("    📝 %s\n", text.Text)
				}
			}
		}
		fmt.Println()
		i++
	}
}

// indent adds a prefix to each line of text.
func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
