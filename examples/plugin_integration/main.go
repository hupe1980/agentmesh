package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/plugin/plugins"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// This example demonstrates plugin integration with AgentMesh using a ReAct agent.
// It shows how plugins automatically intercept model and tool calls without manual context injection.

func main() {
	fmt.Println("=== AgentMesh Plugin Integration Demo ===")
	fmt.Println()

	// Create plugin manager
	pm := callbacks.NewPluginManager()
	defer func() {
		if err := pm.Shutdown(context.Background()); err != nil {
			log.Printf("Warning: plugin shutdown failed: %v", err)
		}
	}()

	// Register logging plugin to track all model and tool invocations
	loggingPlugin := plugins.NewLoggingPlugin(log.Default(), "[Agent]")
	if err := pm.Register(context.Background(), loggingPlugin); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Plugin manager configured with LoggingPlugin")
	fmt.Println()

	// Create a simple calculator tool
	type CalcArgs struct {
		Expression string `json:"expression" jsonschema:"required,description=Math expression to evaluate"`
	}

	calcTool, err := tool.NewFuncTool(
		"calculator",
		"Evaluates mathematical expressions",
		func(ctx context.Context, args CalcArgs) (map[string]any, error) {
			// Simple demo - in production use a proper math evaluator
			result := fmt.Sprintf("Result of %s = 42", args.Expression)
			return map[string]any{"result": result}, nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create OpenAI model (uses OPENAI_API_KEY from environment)
	model := openai.NewModel(
		openai.WithModel("gpt-4o-mini"),
	)

	// Create ReAct agent with plugin manager - callbacks automatically injected!
	reactAgent, err := agent.NewReActAgent(
		model,
		agent.WithTools(calcTool),
		agent.WithPluginManager(pm),
		agent.WithSystemPrompt("You are a helpful math assistant."),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Agent created with automatic plugin injection")
	fmt.Println()

	// Run the agent - plugins automatically intercept all calls
	ctx := context.Background()
	messages := []message.Message{
		message.NewHumanMessageFromText("What is 25 + 17?"),
	}

	fmt.Println("Running agent...")
	fmt.Println()

	result, err := graph.Last(reactAgent.Run(ctx, messages))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("Final response:")
	fmt.Println(message.Stringify(result))
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("The logging plugin automatically tracked:")
	fmt.Println("  - Model invocations (BeforeModel, AfterModel)")
	fmt.Println("  - Tool executions (BeforeTool, AfterTool)")
	fmt.Println("  - No manual context injection required!")
}
