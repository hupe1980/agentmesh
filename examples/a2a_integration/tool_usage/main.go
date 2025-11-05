package main

import (
	"context"
	"log"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	a2atool "github.com/hupe1980/agentmesh/pkg/tool/a2a"
)

func main() {
	ctx := context.Background()

	// Create an A2A tool that calls a remote agent
	// Replace with your actual A2A agent URL
	remoteAgentTool, err := a2atool.NewTool(
		ctx,
		"http://localhost:9001",
		"react",
		a2atool.WithName("remote_agent"),
		a2atool.WithDescription("Call a remote A2A agent for additional processing"),
	)
	if err != nil {
		log.Fatalf("Failed to create A2A tool: %v", err)
	}

	log.Printf("Connected to A2A agent: %s", remoteAgentTool.AgentCard().Name)
	log.Printf("Available skills: %d", len(remoteAgentTool.AgentCard().Skills))

	// Create a local agent that can use the remote A2A agent as a tool
	localAgent, err := agent.NewReActAgent(
		openai.NewModel(),
		agent.WithTools(remoteAgentTool),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Use the agent
	messages := []message.Message{
		message.NewSystemMessageFromText("You are a helpful assistant that can delegate tasks to a remote agent."),
		message.NewHumanMessageFromText("Use the remote agent to generate a greeting for Bob."),
	}

	log.Printf("Executing agent with remote A2A tool...")
	results, err := localAgent.Invoke(ctx, messages)
	if err != nil {
		log.Fatalf("Agent execution failed: %v", err)
	}

	// Print results
	log.Printf("Agent Response:")
	for _, msg := range results {
		if aiMsg, ok := msg.(*message.AIMessage); ok {
			for _, part := range aiMsg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					log.Printf("  %s", textPart.Text)
				}
			}
		}
	}
}
