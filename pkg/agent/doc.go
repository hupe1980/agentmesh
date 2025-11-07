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
compiled, err := agent.NewReActAgent(

	openai.NewModel(),
	weatherTool,

)

// Run agent

	messages := []message.Message{
			message.NewSystemMessageFromText("You are a helpful assistant"),
			message.NewHumanMessageFromText("What's the weather in Boston?"),
		}
		results, err := compiled.Invoke(context.Background(), messages)

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

	stream := compiled.Stream(ctx, messages)
	for event := range stream {
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

	// Struct tool (implements tool.Interface)
	type SearchTool struct{}
	func (s *SearchTool) Name() string { return "search" }
	func (s *SearchTool) Description() string { return "Search the web" }
	func (s *SearchTool) Run(ctx context.Context, input string) (any, error) {
		// Implementation...
		return results, nil
	}

# Configuration

Agents can be configured with options:

compiled, err := agent.NewReActAgent(

	model,
	weatherTool,
	agent.WithMaxIterations(10),

)

# State Management

Agents maintain conversation state automatically:

	// First interaction
	results1, _ := compiled.Invoke(ctx, []message.Message{
		message.NewHumanMessageFromText("What's 2+2?"),
	})

	// Second interaction (includes history)
	results2, _ := compiled.Invoke(ctx, []message.Message{
		message.NewHumanMessageFromText("Add 3 to that"),
	})

# Error Handling

Tool errors are returned to the agent for recovery:

	toolResult := message.NewToolMessage(toolCall.ID, toolCall.Name, err.Error())
	// Agent sees error and can try alternative approach

# Multi-Agent Systems

Combine multiple agents into larger workflows using graph:

	builder := graph.NewBuilder()
	builder.AddNode(&graph.Node{Name: "classifier", RunFunc: classifierAgent})
	builder.AddNode(&graph.Node{Name: "researcher", RunFunc: researchAgent})
	builder.AddNode(&graph.Node{Name: "writer", RunFunc: writerAgent})
	builder.AddConditionalEdges("classifier", routeByCategory)
	// ...

# Supervisor Pattern

Create a supervisor agent that routes tasks to specialized workers:

	// Create specialist agents
	mathAgent, _ := agent.NewReActAgent(model,
		agent.WithSystemPrompt("You are a math expert"),
		agent.WithMaxIterations(5))

	codeAgent, _ := agent.NewReActAgent(model,
		agent.WithSystemPrompt("You are a programming expert"),
		agent.WithMaxIterations(5))

	// Create supervisor with functional options
	supervisor, err := agent.NewSupervisorAgent(model,
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
