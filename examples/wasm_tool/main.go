package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/tool/wasm"
)

func main() {
	ctx := context.Background()

	// Get the directory of the current source file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatalf("Failed to get current file path")
	}
	exampleDir := filepath.Dir(filename)

	// Read the compiled Rust WASM module
	wasmPath := filepath.Join(exampleDir, "calculator.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		log.Fatalf("Failed to read WASM file %s: %v", wasmPath, err)
	}

	// Define the tool schema for LLM integration
	schema := &wasm.ToolSchema{
		Name:        "calculator",
		Description: "Performs arithmetic calculations on mathematical expressions",
		Parameters: &wasm.ParameterSchema{
		Type: "object",
		Properties: map[string]wasm.PropertySchema{
			"expression": {
				Type:        "string",
				Description: "A mathematical expression to evaluate (e.g., '2 + 2', '10 * 5 - 3')",
			},
			},
		Required: []string{"expression"},
		},
	}

	// Create the WASM tool with a compute-only policy (no network, no filesystem)
	tool, err := wasm.NewWASMTool(
		ctx,
		"calculator",
		"Performs arithmetic calculations",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	if err != nil {
		log.Fatalf("Failed to create WASM tool: %v", err)
	}
	defer tool.Close(ctx)

	// Print tool definition (this is what the LLM sees)
	definition := tool.Definition()
	definitionJSON, _ := json.MarshalIndent(definition, "", "  ")
	fmt.Println("Tool Definition:")
	fmt.Println(string(definitionJSON))
	fmt.Println()

	// Test the calculator with direct calls
	fmt.Println("=== Direct Tool Testing ===")
	testCases := []string{
		`{"expression": "2 + 2"}`,
		`{"expression": "10 * 5 + 3"}`,
		`{"expression": "100 / 4"}`,
		`{"expression": "invalid"}`, // Should return error
	}

	for _, testInput := range testCases {
		fmt.Printf("Input:  %s\n", testInput)

		result, err := tool.Call(ctx, testInput)
		if err != nil {
		fmt.Printf("Error:  %v\n", err)
		} else {
		resultJSON, _ := json.MarshalIndent(result, "", "  ")
		fmt.Printf("Output: %s\n", string(resultJSON))
		}
		fmt.Println()
	}

	// Use the tool with a ReAct agent
	fmt.Println("=== ReAct Agent Testing ===")
	fmt.Println()

	reactAgent, err := agent.NewReActAgent(
		openai.NewModel(),
		agent.WithTools(tool),
	)
	if err != nil {
		log.Fatalf("Failed to create ReAct agent: %v", err)
	}

	// Create messages for the agent
	system := message.NewSystemMessageFromText(
		"You are a helpful math assistant. Use the calculator tool to perform arithmetic calculations.",
	)
	human := message.NewHumanMessageFromText("What is 15 multiplied by 8?")

	// Execute the agent
	events, err := graph.Collect(reactAgent.Run(ctx, []message.Message{system, human}))
	if err != nil {
		log.Fatalf("agent execution failed: %v", err)
	}

	// Display the complete conversation transcript
	fmt.Println("Agent Transcript:")
	fmt.Println()
	for i, evt := range events {
		fmt.Printf("[%d] %s\n", i+1, evt.Type())

		switch m := evt.(type) {
		case *message.AIMessage:
			// Display AI's reasoning and responses
		for _, part := range m.Parts() {
		if text, ok := part.(message.TextPart); ok {
				fmt.Printf("    💭 %s\n", text.Text)
			}
			}
			// Show tool calls made by the AI
		if len(m.ToolCalls) > 0 {
		fmt.Printf("    🔧 Tool calls: %v\n", m.ToolCalls)
			}

		case *message.ToolMessage:
			// Display tool execution results
		for _, part := range m.Parts() {
		if text, ok := part.(message.TextPart); ok {
				fmt.Printf("    ⚙️  Tool result: %s\n", text.Text)
			}
			}

		default:
			// Display other message types (system, human)
		for _, part := range m.Parts() {
		if text, ok := part.(message.TextPart); ok {
				fmt.Printf("    📝 %s\n", text.Text)
			}
			}
		}
		fmt.Println()
	}
}
