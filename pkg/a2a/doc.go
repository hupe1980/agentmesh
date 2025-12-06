// Package a2a provides A2A (Agent-to-Agent) Protocol integration for AgentMesh.
//
// This package focuses on:
//   - Protocol message conversion between AgentMesh and A2A formats
//   - Server-side integration (exposing AgentMesh agents as A2A services)
//   - A2A client wrapper for calling remote agents
//
// For using A2A agents within AgentMesh workflows, see:
//   - pkg/tool/a2a: Tool adapter for calling A2A agents from ReAct/RAG agents
//   - pkg/agent: Future home of RemoteAgent/A2AAgent implementations
//
// # Server Integration
//
// Wrap a compiled AgentMesh graph as an A2A agent:
//
//	import (
//	    "github.com/hupe1980/agentmesh/pkg/a2a"
//	    "github.com/hupe1980/agentmesh/pkg/agent"
//	    "github.com/a2aproject/a2a-go/a2agrpc"
//	    "github.com/a2aproject/a2a-go/a2asrv"
//	)
//
//	// Create your agent
//	compiled, _ := agent.NewReAct(model, tools)
//
//	// Wrap as A2A executor
//	executor := a2a.NewExecutor(compiled)
//
//	// Create A2A server
//	requestHandler := a2asrv.NewHandler(executor)
//	grpcHandler := a2agrpc.NewHandler(requestHandler)
//
//	// Serve with gRPC
//	server := grpc.NewServer()
//	grpcHandler.RegisterWith(server)
//	server.Serve(listener)
//
// # Client Usage
//
// Create a client to call remote A2A agents:
//
//	import "github.com/hupe1980/agentmesh/pkg/a2a"
//
//	// Create A2A client
//	client, _ := a2a.NewClient(ctx, "https://agent.example.com", "skill-id")
//
//	// Send a message and get response
//	msg := message.NewHumanMessageFromText("Translate 'hello' to Spanish")
//	responses, _ := client.SendMessage(ctx, msg)
//
//	// Or stream responses
//	for response, err := range client.StreamMessages(ctx, msg) {
//	    // Process each response as it arrives
//	}
//
// # Tool Integration
//
// Use A2A agents as tools in ReAct workflows:
//
//	import a2atool "github.com/hupe1980/agentmesh/pkg/tool/a2a"
//
//	// Create a tool that wraps an A2A agent
//	translatorTool, _ := a2atool.NewTool(ctx, "https://translator.example.com", "translate")
//
//	// Use in a ReAct agent
//	reactAgent, _ := agent.NewReAct(model, agent.WithTools(translatorTool))
//
// # Message Conversion
//
// The package automatically handles conversion between AgentMesh and A2A message formats:
//
//   - AgentMesh message.Message ↔ A2A a2a.Message
//   - Text, tool calls, and artifacts are preserved during conversion
//   - State and metadata are appropriately mapped
//
// # A2A Protocol
//
// For more information about the A2A protocol, visit:
// https://a2a-protocol.org
//
// # Protocol Conversion
//
// Convert between AgentMesh and A2A message formats:
//
//	// AgentMesh -> A2A
//	agentMeshMsg := message.NewHumanMessageFromText("Hello")
//	a2aMsg, _ := a2a.ConvertToA2AMessage(agentMeshMsg)
//
//	// A2A -> AgentMesh
//	messages, _ := a2a.ConvertFromA2AMessage(a2aMsg)
//
// # Related Packages
//
//   - pkg/tool/a2a: Tool adapter for calling A2A agents from ReAct/RAG agents
//   - github.com/a2aproject/a2a-go: Official A2A protocol implementation
//
// # Components
//
// This package provides:
//   - Client: Wrapper for calling remote A2A agents with message conversion
//   - Executor/StreamingExecutor: Server-side integration for exposing agents
//   - Message conversion utilities: ConvertToA2AMessage, ConvertFromA2AMessage
//   - Helper functions: ExtractTextContent, CreateAgentCard, CreateAgentSkill
package a2a
