// Package a2a provides integration between AgentMesh and the Agent-to-Agent (A2A) Protocol.
//
// This package enables AgentMesh agents to:
//   - Expose their functionality as A2A-compliant services (gRPC or JSON-RPC)
//   - Connect to and utilize external A2A agents as tools
//   - Participate in multi-agent collaboration across different systems
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
//	compiled, _ := agent.NewReActAgent(model, tools)
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
// # Client Integration
//
// Use external A2A agents as tools in your AgentMesh workflows:
//
//	// Create a tool that calls an A2A agent
//	a2aTool, _ := a2a.NewAgentTool(ctx, "https://agent.example.com", "skill-id")
//
//	// Use in your agent
//	compiled, _ := agent.NewReActAgent(model, []tool.Tool{a2aTool})
//
// # Graph Node Integration
//
// Add A2A agent nodes directly in your graphs:
//
//	builder := graph.NewBuilder()
//
//	// Add regular nodes
//	builder.AddNodeFunc("prepare", prepareFunc)
//
//	// Add A2A agent node
//	a2aNode := a2a.NewAgentNode("https://agent.example.com", "skill-id")
//	builder.AddNodeFunc("external_agent", a2aNode)
//
//	builder.AddEdge("prepare", "external_agent")
//	builder.AddEdge("external_agent", "END")
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
// # Related Packages
//
// For using A2A agents as tools within AgentMesh workflows, see:
//   - pkg/tool/a2a: Tool adapter for calling A2A agents from ReAct/RAG agents
//
// This package (pkg/a2a) focuses on server infrastructure and protocol bridges,
// while pkg/tool/a2a provides the tool interface for agent consumption.
package a2a
