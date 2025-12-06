// Package main demonstrates integrating LangChainGo models and tools with AgentMesh.
// This example shows:
//   - Using the LangChainGo OpenAI model adapter
//   - Wrapping a LangChainGo prebuilt tool for use with AgentMesh
//   - Building a ReAct agent with LangChainGo components
//
// Run: OPENAI_API_KEY=sk-... go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/message"
	modellc "github.com/hupe1980/agentmesh/pkg/model/langchaingo"
	toollc "github.com/hupe1980/agentmesh/pkg/tool/langchaingo"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable is required")
	}

	llm, err := openai.New(
		openai.WithModel("gpt-4o-mini"),
	)
	if err != nil {
		log.Fatalf("failed to create LangChainGo OpenAI model: %v", err)
	}

	model, err := modellc.NewModel(llm,
		modellc.WithTemperature(0.7),
		modellc.WithMaxTokens(1024),
	)
	if err != nil {
		log.Fatalf("failed to create model adapter: %v", err)
	}

	// Use LangChainGo's built-in Calculator tool
	calc := tools.Calculator{}
	calcTool, err := toollc.NewTool(calc)
	if err != nil {
		log.Fatalf("failed to create calculator tool: %v", err)
	}

	compiled, err := agent.NewReAct(
		model,
		agent.WithTools(calcTool),
	)
	if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

	system := message.NewSystemMessageFromText(
		"You are a helpful math assistant. Use the calculator tool to perform arithmetic calculations.",
	)
	human := message.NewHumanMessageFromText("What is 2 + 2? And then multiply that result by 10.")

	fmt.Println("=== LangChainGo ReAct Agent ===")
	fmt.Println()

	i := 0
	for evt, err := range compiled.Run(context.Background(), []message.Message{system, human}) {
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
				if text, ok := part.(message.TextPart); ok {
					fmt.Printf("    💭 %s\n", text.Text)
				}
			}
			if message.HasToolCalls(evt) {
				fmt.Printf("    🔧 Tool calls: %v\n", m.ToolCalls)
			}

		case *message.ToolMessage:
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Printf("    ⚙️  Tool result: %s\n", text.Text)
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
