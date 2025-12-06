/*
Package agent provides a high-level API for building LLM-powered agents with
tool-calling capabilities, built on top of the graph execution engine.

# Overview

The agent package simplifies the creation of autonomous agents that can:
  - Interact with LLMs (OpenAI, Anthropic, etc.)
  - Use tools to perform actions
  - Maintain conversation history
  - Handle multi-turn interactions automatically

# Quick Start

Create an agent with a model and tools:

	import (
		"context"
		"github.com/hupe1980/agentmesh/pkg/agent"
		"github.com/hupe1980/agentmesh/pkg/model/openai"
		"github.com/hupe1980/agentmesh/pkg/tool"
	)

	// Define a tool
	weatherTool, _ := tool.NewFuncTool(
		"get_weather",
		"Get current weather for a location",
		func(ctx context.Context, location string) (string, error) {
			// Implementation...
			return "Sunny, 72°F", nil
		},
	)

	// Create agent
	compiled, err := agent.NewReAct(
		openai.NewModel(),
		agent.WithTools(weatherTool),
	)

	// Run agent
	messages := []message.Message{
		message.NewHumanMessageFromText("What's the weather in Boston?"),
	}
	lastEvent, err := graph.Last(compiled.Run(context.Background(), messages))

# Architecture

Agents are implemented as graphs with three main nodes:

		START → agent → tools → agent → END
		         ↓              ↑
		         └──────────────┘

	  1. Agent node: Calls LLM to generate response or tool calls
	  2. Tools node: Executes requested tools
	  3. Loop continues until agent produces final response

# Streaming

Get real-time updates as the agent runs:

	seq := compiled.Run(ctx, messages)
	for event, err := range seq {
		if err != nil {
			// Handle error
		}
		switch {
		case event.Node == "agent":
			fmt.Println("Agent thinking...")
		case event.Node == "tools":
			fmt.Println("Executing tools...")
		case event.Err != nil:
			fmt.Printf("Error: %v\n", event.Err)
		}
	}

# Custom Tools

Tools can be functions, structs, or interfaces:

	// Function tool
	calc, _ := tool.NewFuncTool("add", "Add two numbers",
		func(ctx context.Context, a, b int) (int, error) {
			return a + b, nil
		},
	)

	// Struct tool (implements tool.Tool)
	type SearchTool struct{}
	func (s *SearchTool) Name() string { return "search" }
	func (s *SearchTool) Description() string { return "Search the web" }
	func (s *SearchTool) InputSchema() tool.InputSchema { return tool.InputSchema{} }
	func (s *SearchTool) Run(ctx context.Context, input string) (any, error) {
		// Implementation...
		return "results", nil
	}

# Configuration

Agents can be configured with options:

	compiled, err := agent.NewReAct(
		model,
		agent.WithTools(weatherTool),
		agent.WithSupervisorMaxIterations(10),
	)

# State Management

Agents maintain conversation state automatically:

	// First interaction
	msgs1 := []message.Message{
		message.NewHumanMessageFromText("What's 2+2?"),
	}
	result1, _ := graph.Last(compiled.Run(ctx, msgs1))

	// Second interaction (includes history)
	msgs2 := append(msgs1, result1.State[graph.MessagesKeyName].([]message.Message)...)
	msgs2 = append(msgs2, message.NewHumanMessageFromText("Add 3 to that"))
	result2, _ := graph.Last(compiled.Run(ctx, msgs2))

# Error Handling

Tool errors are returned to the agent for recovery:

	toolResult := message.NewToolMessage(toolCall.ID, toolCall.Name, err.Error())
	// Agent sees error and can try alternative approach

# Multi-Agent Systems

Combine multiple agents into larger workflows using message graph:

	g := message.NewGraph()
	g.CommandNode("classifier", classifierAgent, "researcher", "writer")
	g.AgentNode("researcher", researchAgent, "writer")
	g.AgentNode("writer", writerAgent, graph.END)
	g.Start("classifier")
	// ...

# Supervisor Pattern

Create a supervisor agent that routes tasks to specialized workers:

	// Create specialist agents
	mathAgent, _ := agent.NewReAct(model,
		agent.WithSystemPrompt("You are a math expert"),
		agent.WithMaxIterations(5))

	codeAgent, _ := agent.NewReAct(model,
		agent.WithSystemPrompt("You are a programming expert"),
		agent.WithMaxIterations(5))

	// Create supervisor with functional options
	supervisor, err := agent.NewSupervisor(model,
		agent.WithWorker("math", "Math expert", mathAgent),
		agent.WithWorker("code", "Programming expert", codeAgent),
		agent.WithSupervisorSystemPrompt("Route to specialists"),
		agent.WithSupervisorMaxIterations(10),
		agent.WithWorkerContext(false), // Fresh context for each task
		agent.WithWorkerRetries(2))

The supervisor automatically creates handoff tools for each worker and routes
tasks to the most appropriate specialist based on the query.

See examples/supervisor_simple for a complete example.
*/
package agent
